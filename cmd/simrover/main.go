// Command simrover simulates one or many NTRIP rovers (clients) that connect
// to the caster and receive RTCM3 frames.
//
// Single rover:   simrover -mount RTCM_01 -user rover1 -pass test
// Stress test:    simrover -mount RTCM_01 -count 5000 -ramp 2ms
// Multi mount:    simrover -mounts BENCH_0,BENCH_1,BENCH_2 -count 3000 -ramp 2ms
//
// In multi-mount mode, rovers are distributed round-robin across mountpoints.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	gTotalBytes   int64
	gTotalPkts    int64
	gConnected    int64
	gFailed       int64
	gDisconnected int64
	gKicked       int64
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2101", "caster address")
	mount := flag.String("mount", "", "single mountpoint name")
	mounts := flag.String("mounts", "", "comma-separated mountpoint list (rovers distributed round-robin)")
	user := flag.String("user", "", "username for Basic Auth")
	pass := flag.String("pass", "", "password")
	count := flag.Int("count", 1, "number of concurrent rover connections")
	rampDelay := flag.Duration("ramp", 5*time.Millisecond, "delay between each connection during ramp-up")
	quiet := flag.Bool("quiet", false, "suppress per-packet logs (auto-enabled when count > 10)")
	flag.Parse()

	mountList := buildMountList(*mount, *mounts)
	if len(mountList) == 0 {
		log.Fatal("specify -mount or -mounts")
	}

	if *count > 10 {
		*quiet = true
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	stopCh := make(chan struct{})
	go func() {
		<-sigCh
		close(stopCh)
	}()

	start := time.Now()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				printStats(start)
			}
		}
	}()

	var wg sync.WaitGroup

	if len(mountList) == 1 {
		log.Printf("launching %d rover(s) → %s/%s (ramp=%v)", *count, *addr, mountList[0], *rampDelay)
	} else {
		log.Printf("launching %d rover(s) → %s/{%s} (ramp=%v)", *count, *addr, strings.Join(mountList, ","), *rampDelay)
	}

	for i := 0; i < *count; i++ {
		mp := mountList[i%len(mountList)]
		wg.Add(1)
		go func(id int, mountName string) {
			defer wg.Done()
			runRover(id, *addr, mountName, *user, *pass, *quiet, stopCh)
		}(i, mp)

		if i < *count-1 {
			select {
			case <-stopCh:
				break
			case <-time.After(*rampDelay):
			}
		}
	}

	<-stopCh
	log.Println("shutting down, waiting for all rovers to exit...")
	wg.Wait()

	printStats(start)
	log.Println("done.")
}

func buildMountList(single, multi string) []string {
	if multi != "" {
		parts := strings.Split(multi, ",")
		var list []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				list = append(list, p)
			}
		}
		return list
	}
	if single != "" {
		return []string{single}
	}
	return nil
}

func runRover(id int, addr, mount, user, pass string, quiet bool, stopCh <-chan struct{}) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		atomic.AddInt64(&gFailed, 1)
		if !quiet {
			log.Printf("[rover-%d] dial failed: %v", id, err)
		}
		return
	}
	defer func() {
		conn.Close()
		atomic.AddInt64(&gDisconnected, 1)
	}()

	if err := sendRoverRequest(conn, mount, user, pass); err != nil {
		atomic.AddInt64(&gFailed, 1)
		if !quiet {
			log.Printf("[rover-%d] send request failed: %v", id, err)
		}
		return
	}

	resp := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(resp)
	if err != nil {
		atomic.AddInt64(&gFailed, 1)
		if !quiet {
			log.Printf("[rover-%d] read response failed: %v", id, err)
		}
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	respStr := string(resp[:n])
	if !strings.Contains(respStr, "200") {
		atomic.AddInt64(&gFailed, 1)
		if !quiet {
			log.Printf("[rover-%d] rejected: %s", id, strings.TrimSpace(respStr))
		}
		return
	}

	atomic.AddInt64(&gConnected, 1)
	if !quiet {
		log.Printf("[rover-%d/%s] connected", id, mount)
	}

	var leftover []byte
	headerEnd := strings.Index(respStr, "\r\n\r\n")
	if headerEnd >= 0 && headerEnd+4 < n {
		leftover = resp[headerEnd+4 : n]
	}

	framer := &rtcmFramer{}

	if len(leftover) > 0 {
		pkts := framer.push(leftover)
		atomic.AddInt64(&gTotalBytes, int64(len(leftover)))
		atomic.AddInt64(&gTotalPkts, int64(len(pkts)))
		if !quiet {
			for _, p := range pkts {
				logPacket(id, p)
			}
		}
	}

	go func() {
		<-stopCh
		conn.Close()
	}()

	buf := make([]byte, 4096)
	for {
		rn, rerr := conn.Read(buf)
		if rn > 0 {
			atomic.AddInt64(&gTotalBytes, int64(rn))
			pkts := framer.push(buf[:rn])
			atomic.AddInt64(&gTotalPkts, int64(len(pkts)))
			if !quiet {
				for _, p := range pkts {
					logPacket(id, p)
				}
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				select {
				case <-stopCh:
				default:
					atomic.AddInt64(&gKicked, 1)
					if !quiet {
						log.Printf("[rover-%d] read error: %v", id, rerr)
					}
				}
			}
			return
		}
	}
}

func printStats(start time.Time) {
	elapsed := time.Since(start)
	bytes := atomic.LoadInt64(&gTotalBytes)
	log.Printf("[aggregate] connected=%d failed=%d disconnected=%d kicked=%d | pkts=%d bytes=%d rate=%.1f KB/s | %s",
		atomic.LoadInt64(&gConnected),
		atomic.LoadInt64(&gFailed),
		atomic.LoadInt64(&gDisconnected),
		atomic.LoadInt64(&gKicked),
		atomic.LoadInt64(&gTotalPkts),
		bytes,
		float64(bytes)/1024/elapsed.Seconds(),
		elapsed.Round(time.Second))
}

func sendRoverRequest(conn net.Conn, mount, user, pass string) error {
	req := fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: localhost\r\nNtrip-Version: Ntrip/2.0\r\nUser-Agent: NTRIP simrover/1.0\r\n", mount)
	if user != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		req += "Authorization: Basic " + cred + "\r\n"
	}
	req += "\r\n"
	_, err := io.WriteString(conn, req)
	return err
}

func logPacket(id int, data []byte) {
	if len(data) < 5 {
		log.Printf("[rover-%d] RTCM frame: %d bytes (too short)", id, len(data))
		return
	}
	msgType := uint16(data[3])<<4 | uint16(data[4])>>4
	log.Printf("[rover-%d] RTCM msg=%4d len=%d", id, msgType, len(data))
}

// --- Minimal RTCM3 framer (mirrors internal/rtcm but avoids import) ---

const (
	preamble  = 0xD3
	headerLen = 3
	crcLen    = 3
)

type rtcmFramer struct {
	buf []byte
}

func (f *rtcmFramer) push(data []byte) [][]byte {
	f.buf = append(f.buf, data...)
	var frames [][]byte
	for {
		if len(f.buf) < headerLen {
			break
		}
		if f.buf[0] != preamble {
			idx := -1
			for i, b := range f.buf {
				if b == preamble {
					idx = i
					break
				}
			}
			if idx < 0 {
				f.buf = f.buf[:0]
				break
			}
			f.buf = f.buf[idx:]
			continue
		}
		payloadLen := int(f.buf[1]&0x03)<<8 | int(f.buf[2])
		frameLen := headerLen + payloadLen + crcLen
		if len(f.buf) < frameLen {
			break
		}
		frame := make([]byte, frameLen)
		copy(frame, f.buf[:frameLen])
		frames = append(frames, frame)
		f.buf = f.buf[frameLen:]
	}
	return frames
}
