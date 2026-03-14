package rtcm

// RTCM3 frame layout:
//   byte 0:       0xD3 (preamble)
//   byte 1-2:     reserved(6bit) + length(10bit)
//   byte 3..3+L:  payload (L bytes)
//   last 3 bytes: CRC-24Q
//
// Total frame size = 3 (header) + L (payload) + 3 (CRC) = L + 6

const (
	preamble   = 0xD3
	headerLen  = 3
	crcLen     = 3
	maxPayload = 1023 // 10-bit length field
	maxFrame   = headerLen + maxPayload + crcLen
)

// RTCMFramer accumulates raw bytes from a TCP stream and extracts complete
// RTCM3 frames. It is not safe for concurrent use.
type RTCMFramer struct {
	buf []byte
}

// Push feeds raw bytes into the framer and returns zero or more complete
// RTCM3 packets. Each returned RTCMPacket owns its own slice and is safe
// for shared, read-only broadcast.
func (f *RTCMFramer) Push(data []byte) []*RTCMPacket {
	f.buf = append(f.buf, data...)

	var packets []*RTCMPacket
	for {
		if len(f.buf) < headerLen {
			break
		}

		// Scan for preamble.
		if f.buf[0] != preamble {
			if idx := findPreamble(f.buf); idx < 0 {
				f.buf = f.buf[:0]
				break
			} else {
				f.buf = f.buf[idx:]
				continue
			}
		}

		payloadLen := int(f.buf[1]&0x03)<<8 | int(f.buf[2])
		frameLen := headerLen + payloadLen + crcLen

		if len(f.buf) < frameLen {
			break
		}

		pkt := make([]byte, frameLen)
		copy(pkt, f.buf[:frameLen])
		packets = append(packets, &RTCMPacket{Data: pkt})
		f.buf = f.buf[frameLen:]
	}

	return packets
}

// Reset discards any buffered data.
func (f *RTCMFramer) Reset() {
	f.buf = f.buf[:0]
}

func findPreamble(buf []byte) int {
	for i, b := range buf {
		if b == preamble {
			return i
		}
	}
	return -1
}
