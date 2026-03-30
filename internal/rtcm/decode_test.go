package rtcm

import "testing"

func TestExtractMsgType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{
			name: "1005 message",
			// RTCM3 frame: 0xD3 | len(10bit) | payload | CRC
			// 1005 payload starts with msg type (12 bit)
			// DF002 = 12 bits: byte 3 = high 8 bits, byte 4 high nibble = low 4 bits
			// For 1005 (0x3ED): byte 3 = 0x3E, byte 4 high nibble = 0xD
			data: []byte{
				0xD3, 0x00, 0x13, // header: preamble + length (19 bytes payload)
				0x3E, 0xD0, // payload byte 0-1: msg type 1005 (0x3ED) + station ID high nibble
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // more payload
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // more payload
				0x00, 0x00, 0x00, // CRC (dummy)
			},
			expected: 1005,
		},
		{
			name: "1074 message",
			// 1074 = 0x432 = 0b0100_0011_0010
			// byte 3 = 0x43, byte 4 high nibble = 0x2, so byte 4 = 0x2X
			data: []byte{
				0xD3, 0x00, 0x10, // header: preamble + length (16 bytes payload)
				0x43, 0x20, // msg type 1074 (0x432)
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // more payload
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // more payload
				0x00, 0x00, 0x00, // CRC (dummy)
			},
			expected: 1074,
		},
		{
			name:     "too short - no payload",
			data:     []byte{0xD3, 0x00},
			expected: 0,
		},
		{
			name:     "too short - one byte payload",
			data:     []byte{0xD3, 0x00, 0x01, 0x03},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &RTCMPacket{Data: tt.data}
			got := ExtractMsgType(pkt)
			if got != tt.expected {
				t.Errorf("ExtractMsgType() = %d, want %d", got, tt.expected)
			}
		})
	}
}