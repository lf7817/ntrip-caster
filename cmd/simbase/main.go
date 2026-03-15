// Command simbase simulates one or many NTRIP base stations (sources) that
// connect to the caster and stream synthetic RTCM3 frames.
//
// Single base:   simbase -mount RTCM_01 -user base -pass test
// Multi base:    simbase -count 5 -mount-prefix BENCH -user base -pass test
//
// In multi mode, base i connects to mountpoint "{prefix}_{i}" (e.g. BENCH_0 … BENCH_4).
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	gTotalBytes int64
	gTotalPkts  int64
	gConnected  int64
	gFailed     int64
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2101", "caster address")
	mount := flag.String("mount", "", "mountpoint name (single-base mode)")
	mountPrefix := flag.String("mount-prefix", "BENCH", "mountpoint name prefix (multi-base mode, e.g. BENCH → BENCH_0, BENCH_1, ...)")
	user := flag.String("user", "", "username for Basic Auth (Rev2)")
	pass := flag.String("pass", "", "password")
	rev := flag.Int("rev", 2, "NTRIP revision: 1 or 2")
	interval := flag.Duration("interval", 1*time.Second, "send interval per RTCM frame")
	msgType := flag.Int("msgtype", 1005, "RTCM3 message type to simulate")
	payloadSize := flag.Int("size", 19, "RTCM3 payload size in bytes")
	count := flag.Int("count", 1, "number of base stations (each on its own mountpoint)")
	flag.Parse()

	// Build mountpoint name list.
	mounts := buildMountList(*mount, *mountPrefix, *count)

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

	if *count == 1 {
		log.Printf("starting 1 base → %s/%s", *addr, mounts[0])
	} else {
		log.Printf("starting %d bases → %s/{%s … %s}", *count, *addr, mounts[0], mounts[len(mounts)-1])
	}

	for i, mp := range mounts {
		wg.Add(1)
		go func(id int, mountName string) {
			defer wg.Done()
			runBase(id, *addr, mountName, *user, *pass, *rev, *interval, uint16(*msgType), *payloadSize, stopCh)
		}(i, mp)
	}

	<-stopCh
	wg.Wait()
	printStats(start)
	log.Println("done.")
}

func buildMountList(mount, prefix string, count int) []string {
	if count == 1 && mount != "" {
		return []string{mount}
	}
	if count == 1 && mount == "" {
		return []string{prefix + "_0"}
	}
	mounts := make([]string, count)
	for i := range mounts {
		mounts[i] = fmt.Sprintf("%s_%d", prefix, i)
	}
	return mounts
}

func runBase(id int, addr, mount, user, pass string, rev int, interval time.Duration, msgType uint16, payloadSize int, stopCh <-chan struct{}) {
	label := fmt.Sprintf("[base-%d/%s]", id, mount)

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		atomic.AddInt64(&gFailed, 1)
		log.Printf("%s dial failed: %v", label, err)
		return
	}
	defer conn.Close()

	if err := sendSourceRequest(conn, rev, mount, user, pass); err != nil {
		atomic.AddInt64(&gFailed, 1)
		log.Printf("%s send request failed: %v", label, err)
		return
	}

	resp := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(resp)
	if err != nil {
		atomic.AddInt64(&gFailed, 1)
		log.Printf("%s read response failed: %v", label, err)
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	respStr := string(resp[:n])
	if !strings.Contains(respStr, "200") {
		atomic.AddInt64(&gFailed, 1)
		log.Printf("%s rejected: %s", label, strings.TrimSpace(respStr))
		return
	}

	atomic.AddInt64(&gConnected, 1)
	log.Printf("%s connected", label)

	// close conn when signal received
	go func() {
		<-stopCh
		conn.Close()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			frame := buildRTCM3Frame(msgType, payloadSize)
			if _, err := conn.Write(frame); err != nil {
				log.Printf("%s write error: %v", label, err)
				return
			}
			atomic.AddInt64(&gTotalBytes, int64(len(frame)))
			atomic.AddInt64(&gTotalPkts, 1)
		}
	}
}

func printStats(start time.Time) {
	elapsed := time.Since(start)
	bytes := atomic.LoadInt64(&gTotalBytes)
	log.Printf("[aggregate] bases connected=%d failed=%d | pkts=%d bytes=%d rate=%.1f KB/s | %s",
		atomic.LoadInt64(&gConnected),
		atomic.LoadInt64(&gFailed),
		atomic.LoadInt64(&gTotalPkts),
		bytes,
		float64(bytes)/1024/elapsed.Seconds(),
		elapsed.Round(time.Second))
}

func sendSourceRequest(conn net.Conn, rev int, mount, user, pass string) error {
	var req string
	switch rev {
	case 1:
		req = fmt.Sprintf("SOURCE %s /%s\r\n\r\n", pass, mount)
	case 2:
		req = fmt.Sprintf("POST /%s HTTP/1.1\r\nHost: localhost\r\nNtrip-Version: Ntrip/2.0\r\nTransfer-Encoding: chunked\r\n", mount)
		if user != "" {
			cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
			req += "Authorization: Basic " + cred + "\r\n"
		}
		req += "\r\n"
	default:
		return fmt.Errorf("unsupported rev %d", rev)
	}
	_, err := io.WriteString(conn, req)
	return err
}

// buildRTCM3Frame creates a valid RTCM3 frame with proper CRC-24Q.
func buildRTCM3Frame(msgType uint16, payloadLen int) []byte {
	if payloadLen < 2 {
		payloadLen = 2
	}
	if payloadLen > 1023 {
		payloadLen = 1023
	}

	frame := make([]byte, 3+payloadLen+3)

	frame[0] = 0xD3
	frame[1] = byte((payloadLen >> 8) & 0x03)
	frame[2] = byte(payloadLen & 0xFF)

	frame[3] = byte(msgType >> 4)
	frame[4] = byte(msgType<<4) | byte(rand.Intn(16))

	for i := 5; i < 3+payloadLen; i++ {
		frame[i] = byte(rand.Intn(256))
	}

	crc := crc24q(frame[:3+payloadLen])
	frame[3+payloadLen] = byte(crc >> 16)
	frame[3+payloadLen+1] = byte(crc >> 8)
	frame[3+payloadLen+2] = byte(crc)

	return frame
}

func crc24q(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc ^= uint32(b) << 16
		for i := 0; i < 8; i++ {
			crc <<= 1
			if crc&0x1000000 != 0 {
				crc ^= 0x1864CFB
			}
		}
	}
	return crc & 0xFFFFFF
}
