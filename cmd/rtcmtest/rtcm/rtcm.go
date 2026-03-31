package rtcm

import (
	"errors"
	"fmt"
)

// =========================
// 数据结构
// =========================

type Message1005 struct {
	StationID int
	ITRF      int
	X         float64 // meters
	Y         float64
	Z         float64
}

// =========================
// 帧解析
// =========================

func ParseRTCMFrame(data []byte) ([]byte, error) {
	if len(data) < 6 {
		return nil, errors.New("frame too short")
	}

	if data[0] != 0xD3 {
		return nil, errors.New("invalid preamble")
	}

	length := int(data[1]&0x03)<<8 | int(data[2])

	if len(data) < 3+length+3 {
		return nil, errors.New("incomplete frame")
	}

	payload := data[3 : 3+length]

	// TODO: CRC 校验（可加）

	return payload, nil
}

// =========================
// bit 操作
// =========================

func getBits(buf []byte, pos int, length int) uint64 {
	var val uint64
	for i := 0; i < length; i++ {
		byteIndex := (pos + i) / 8
		bitIndex := 7 - ((pos + i) % 8)
		bit := (buf[byteIndex] >> bitIndex) & 1
		val = (val << 1) | uint64(bit)
	}
	return val
}

func getBitsSigned(buf []byte, pos int, length int) int64 {
	val := int64(getBits(buf, pos, length))

	// 补码处理
	if (val & (1 << (length - 1))) != 0 {
		val -= (1 << length)
	}
	return val
}

// =========================
// 1005 解析
// =========================

func Decode1005(payload []byte) (*Message1005, error) {
	pos := 0

	msgType := getBits(payload, pos, 12)
	pos += 12

	if msgType != 1005 {
		return nil, fmt.Errorf("not 1005, got %d", msgType)
	}

	stationID := int(getBits(payload, pos, 12))
	pos += 12

	itrf := int(getBits(payload, pos, 6))
	pos += 6

	// flags（4bit）
	pos += 4

	// X
	x := getBitsSigned(payload, pos, 38)
	pos += 38

	pos += 1 // skip

	// Y
	y := getBitsSigned(payload, pos, 38)
	pos += 38

	pos += 1

	// Z
	z := getBitsSigned(payload, pos, 38)
	pos += 38

	// 转单位（0.0001 m）
	return &Message1005{
		StationID: stationID,
		ITRF:      itrf,
		X:         float64(x) * 0.0001,
		Y:         float64(y) * 0.0001,
		Z:         float64(z) * 0.0001,
	}, nil
}

// =========================
// 对外入口
// =========================

func Parse1005(data []byte) (*Message1005, error) {
	payload, err := ParseRTCMFrame(data)
	if err != nil {
		return nil, err
	}

	return Decode1005(payload)
}
