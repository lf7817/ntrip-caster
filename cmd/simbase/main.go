// Command simbase simulates an NTRIP base station (source) that connects
// to the caster and streams synthetic RTCM3 frames at a configurable rate.
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
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2101", "caster address")
	mount := flag.String("mount", "TEST", "mountpoint name")
	user := flag.String("user", "", "username for Basic Auth (Rev2)")
	pass := flag.String("pass", "", "password")
	rev := flag.Int("rev", 2, "NTRIP revision: 1 or 2")
	interval := flag.Duration("interval", 1*time.Second, "send interval per RTCM frame")
	msgType := flag.Int("msgtype", 1005, "RTCM3 message type to simulate")
	payloadSize := flag.Int("size", 19, "RTCM3 payload size in bytes")
	flag.Parse()

	conn, err := net.DialTimeout("tcp", *addr, 5*time.Second)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()

	if err := sendSourceRequest(conn, *rev, *mount, *user, *pass); err != nil {
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

	var totalBytes int64
	var totalPkts int64

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	statTicker := time.NewTicker(5 * time.Second)
	defer statTicker.Stop()

	start := time.Now()

	for {
		select {
		case <-sigCh:
			elapsed := time.Since(start)
			log.Printf("stopped. sent %d packets, %d bytes in %s (%.1f B/s)",
				totalPkts, totalBytes, elapsed.Round(time.Millisecond),
				float64(totalBytes)/elapsed.Seconds())
			return

		case <-ticker.C:
			frame := buildRTCM3Frame(uint16(*msgType), *payloadSize)
			if _, err := conn.Write(frame); err != nil {
				log.Fatalf("write: %v", err)
			}
			atomic.AddInt64(&totalBytes, int64(len(frame)))
			atomic.AddInt64(&totalPkts, 1)

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
//
// Frame layout:
//
//	[0xD3] [reserved(6b)+length(10b)] [payload...] [CRC-24Q(3B)]
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

	// First 12 bits of payload = message type number
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

// crc24q computes the CRC-24Q checksum used by RTCM3.
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
