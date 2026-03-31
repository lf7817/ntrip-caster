package main

import (
	"encoding/hex"
	"fmt"
	"math"
)

// =====================
// CRC24Q
// =====================

var crc24qTable = [256]uint32{
	0x000000, 0x864CFB, 0x8AD50D, 0x0C99F6, 0x93E6E1, 0x15AA1A, 0x1933EC, 0x9F7F17,
	// 省略（下面给完整版本）
}

func init() {
	const poly = 0x1864CFB
	for i := 0; i < 256; i++ {
		crc := uint32(i) << 16
		for j := 0; j < 8; j++ {
			if (crc & 0x800000) != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		crc24qTable[i] = crc & 0xFFFFFF
	}
}

func crc24q(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = ((crc << 8) & 0xFFFFFF) ^ crc24qTable[(crc>>16)^uint32(b)]
	}
	return crc
}

// =====================
// bit 操作（核心）
// =====================

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
	if (val & (1 << (length - 1))) != 0 {
		val -= (1 << length)
	}
	return val
}

// =====================
// RTCM 1005 解析
// =====================

func parse1005(payload []byte) (float64, float64, float64, int, error) {
	pos := 0

	msgType := getBits(payload, pos, 12)
	pos += 12
	if msgType != 1005 {
		return 0, 0, 0, 0, fmt.Errorf("not 1005")
	}

	stationID := int(getBits(payload, pos, 12))
	pos += 12

	itrf := getBits(payload, pos, 6)
	pos += 6

	// flags（4bit）
	gps := getBits(payload, pos, 1)
	pos++
	glo := getBits(payload, pos, 1)
	pos++
	gal := getBits(payload, pos, 1)
	pos++
	ref := getBits(payload, pos, 1)
	pos++

	_ = gps
	_ = glo
	_ = gal
	_ = ref
	_ = itrf

	// X
	x := getBitsSigned(payload, pos, 38)
	pos += 38

	pos++ // skip

	// Y
	y := getBitsSigned(payload, pos, 38)
	pos += 38

	pos++

	// Z
	z := getBitsSigned(payload, pos, 38)
	pos += 38

	X := float64(x) * 0.0001
	Y := float64(y) * 0.0001
	Z := float64(z) * 0.0001

	return X, Y, Z, stationID, nil
}

// =====================
// ECEF -> 经纬度
// =====================

func ecefToLLA(x, y, z float64) (lat, lon, h float64) {
	a := 6378137.0
	e2 := 0.00669437999014

	lon = math.Atan2(y, x)

	p := math.Sqrt(x*x + y*y)
	lat = math.Atan2(z, p*(1-e2))

	for i := 0; i < 5; i++ {
		N := a / math.Sqrt(1-e2*math.Sin(lat)*math.Sin(lat))
		h = p/math.Cos(lat) - N
		lat = math.Atan2(z, p*(1-e2*(N/(N+h))))
	}

	lat = lat * 180 / math.Pi
	lon = lon * 180 / math.Pi
	return
}

// =====================
// 主程序
// =====================

func main() {
	hexStr := "D300133ED00003B9E80CDA018B0AF8131607D02B03AC938352"

	data, _ := hex.DecodeString(hexStr)

	// 校验长度
	length := int(data[1]&0x03)<<8 | int(data[2])
	payload := data[3 : 3+length]

	// CRC 校验
	crcCalc := crc24q(data[:3+length])
	crcRecv := uint32(data[3+length])<<16 |
		uint32(data[3+length+1])<<8 |
		uint32(data[3+length+2])

	fmt.Printf("CRC OK: %v\n", crcCalc == crcRecv)

	X, Y, Z, stationID, err := parse1005(payload)
	if err != nil {
		panic(err)
	}

	fmt.Printf("StationID: %d\n", stationID)
	fmt.Printf("ECEF:\nX=%.3f\nY=%.3f\nZ=%.3f\n", X, Y, Z)

	lat, lon, h := ecefToLLA(X, Y, Z)

	fmt.Printf("\nWGS84:\nLat=%.6f\nLon=%.6f\nH=%.2f\n", lat, lon, h)
}
