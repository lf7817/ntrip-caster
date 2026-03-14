// Command simrover simulates an NTRIP rover (client) that connects to the
// caster and receives RTCM3 frames from a mountpoint.
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
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2101", "caster address")
	mount := flag.String("mount", "TEST", "mountpoint name")
	user := flag.String("user", "", "username for Basic Auth")
	pass := flag.String("pass", "", "password")
	flag.Parse()

	conn, err := net.DialTimeout("tcp", *addr, 5*time.Second)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()

	if err := sendRoverRequest(conn, *mount, *user, *pass); err != nil {
		log.Fatalf("send request: %v", err)
	}

	resp := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(resp)
	if err != nil {
		log.Fatalf("read response: %v", err)
	}
	_ = conn.SetReadDeadline(time.Time{})

	respStr := string(resp[:n])
	if !strings.Contains(respStr, "200") {
		log.Fatalf("caster rejected connection: %s", strings.TrimSpace(respStr))
	}
	log.Printf("connected to %s/%s (%s)", *addr, *mount, strings.TrimSpace(respStr))

	// Any data after the HTTP response header is already RTCM.
	var leftover []byte
	headerEnd := strings.Index(respStr, "\r\n\r\n")
	if headerEnd >= 0 && headerEnd+4 < n {
		leftover = resp[headerEnd+4 : n]
	}

	var totalBytes int64
	var totalPkts int64

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	stopCh := make(chan struct{})
	go func() {
		<-sigCh
		close(stopCh)
		conn.Close()
	}()

	statTicker := time.NewTicker(5 * time.Second)
	defer statTicker.Stop()

	go func() {
		start := time.Now()
		for {
			select {
			case <-stopCh:
				return
			case <-statTicker.C:
				elapsed := time.Since(start)
				pkts := atomic.LoadInt64(&totalPkts)
				bytes := atomic.LoadInt64(&totalBytes)
				log.Printf("[stats] %d pkts, %d bytes, %.1f B/s, running %s",
					pkts, bytes,
					float64(bytes)/elapsed.Seconds(),
					elapsed.Round(time.Second))
			}
		}
	}()

	framer := &rtcmFramer{}

	if len(leftover) > 0 {
		pkts := framer.push(leftover)
		for _, p := range pkts {
			logPacket(p)
		}
		atomic.AddInt64(&totalBytes, int64(len(leftover)))
		atomic.AddInt64(&totalPkts, int64(len(pkts)))
	}

	start := time.Now()
	buf := make([]byte, 4096)
	for {
		rn, rerr := conn.Read(buf)
		if rn > 0 {
			atomic.AddInt64(&totalBytes, int64(rn))
			pkts := framer.push(buf[:rn])
			for _, p := range pkts {
				logPacket(p)
				atomic.AddInt64(&totalPkts, 1)
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				select {
				case <-stopCh:
				default:
					log.Printf("read error: %v", rerr)
				}
			}
			break
		}
	}

	elapsed := time.Since(start)
	log.Printf("disconnected. received %d packets, %d bytes in %s (%.1f B/s)",
		atomic.LoadInt64(&totalPkts),
		atomic.LoadInt64(&totalBytes),
		elapsed.Round(time.Millisecond),
		float64(atomic.LoadInt64(&totalBytes))/elapsed.Seconds())
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

func logPacket(data []byte) {
	if len(data) < 5 {
		log.Printf("  RTCM frame: %d bytes (too short to decode type)", len(data))
		return
	}
	msgType := uint16(data[3])<<4 | uint16(data[4])>>4
	log.Printf("  RTCM msg=%4d  len=%d bytes", msgType, len(data))
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
