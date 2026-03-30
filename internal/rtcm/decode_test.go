package rtcm

import (
	"math"
	"testing"
)

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

func TestEcefToLatLng(t *testing.T) {
	tests := []struct {
		name         string
		x, y, z      float64
		wantLat      float64 // 度
		wantLon      float64 // 度
		wantH        float64 // 米
		toleranceLat float64 // 度容差
		toleranceLon float64 // 度容差
		toleranceH    float64 // 米容差
		wantErr      bool
	}{
		{
			// 北极点（Z 轴方向）
			// 注意：WGS84 椭球在极点半径约 6356752m，所以 z=6378137 时高度约 21385m
			name:         "North Pole",
			x:            0,
			y:            0,
			z:            6378137.0,
			wantLat:      90.0,
			wantLon:      0.0,
			wantH:        21385,
			toleranceLat: 0.001,
			toleranceLon: 0.001,
			toleranceH:   10,
		},
		{
			// 赤道本初子午线（X 轴方向）
			name:         "Equator Prime Meridian",
			x:            6378137.0,
			y:            0,
			z:            0,
			wantLat:      0.0,
			wantLon:      0.0,
			wantH:        0,
			toleranceLat: 0.001,
			toleranceLon: 0.001,
			toleranceH:   1,
		},
		{
			// 赤道东经 90°（Y 轴方向）
			name:         "Equator 90E",
			x:            0,
			y:            6378137.0,
			z:            0,
			wantLat:      0.0,
			wantLon:      90.0,
			wantH:        0,
			toleranceLat: 0.001,
			toleranceLon: 0.001,
			toleranceH:   1,
		},
		{
			// 北京（粗略坐标，高度约为 1471m）
			name:         "Beijing (approximate)",
			x:            -2187800,
			y:            4385000,
			z:            4071000,
			wantLat:      39.9,
			wantLon:      116.4,
			wantH:        1471,
			toleranceLat: 0.5,
			toleranceLon: 0.5,
			toleranceH:   100,
		},
		{
			// 全零坐标（应返回错误）
			name:    "Zero coordinates",
			x:       0,
			y:       0,
			z:       0,
			wantErr: true,
		},
		{
			// 超出地球范围（应返回错误）
			name:    "Out of range - too far",
			x:       10000000,
			y:       0,
			z:       0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, h, err := ecefToLatLng(tt.x, tt.y, tt.z)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ecefToLatLng() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ecefToLatLng() unexpected error: %v", err)
				return
			}

			if diff := math.Abs(lat - tt.wantLat); diff > tt.toleranceLat {
				t.Errorf("lat = %.6f, want %.6f (diff %.6f)", lat, tt.wantLat, diff)
			}
			if diff := math.Abs(lon - tt.wantLon); diff > tt.toleranceLon {
				t.Errorf("lon = %.6f, want %.6f (diff %.6f)", lon, tt.wantLon, diff)
			}
			if diff := math.Abs(h - tt.wantH); diff > tt.toleranceH {
				t.Errorf("h = %.2f, want %.2f (diff %.2f)", h, tt.wantH, diff)
			}
		})
	}
}