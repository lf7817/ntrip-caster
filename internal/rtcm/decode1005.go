package rtcm

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// WGS84 椭球参数
const (
	wgs84A   = 6378137.0              // 长半轴 (m)
	wgs84F   = 1 / 298.257223563      // 扁率
	wgs84B   = wgs84A * (1 - wgs84F)  // 短半轴 (m)
	wgs84E2  = 2*wgs84F - wgs84F*wgs84F // 第一偏心率平方
	wgs84Ep2 = wgs84E2 / (1 - wgs84E2)  // 第二偏心率平方
)

// 错误定义
var (
	ErrZeroCoordinates = errors.New("ECEF coordinates are all zero")
	ErrOutOfRange      = errors.New("ECEF coordinates out of valid Earth range")
)

// AntennaPosition 表示基站天线位置（经纬度格式）。
type AntennaPosition struct {
	Latitude  float64 // 度，范围 [-90, 90]
	Longitude float64 // 度，范围 [-180, 180]
	Height    float64 // 米，天线大地高
	UpdatedAt int64   // Unix 秒时间戳
}

// ecefToLatLng 将 ECEF 坐标转换为 WGS84 经纬度。
// 使用迭代法求解纬度，精度约 1mm。
func ecefToLatLng(x, y, z float64) (lat, lon, h float64, err error) {
	// 检查输入有效性
	if x == 0 && y == 0 && z == 0 {
		return 0, 0, 0, ErrZeroCoordinates
	}

	// 检查是否在地球范围内（放宽范围以允许不同数据源）
	r := math.Sqrt(x*x + y*y + z*z)
	// 地球半径约 6371 km，但允许更宽的范围（可能是缩放数据或相对坐标）
	if r < 1000000 || r > 10000000 {
		return 0, 0, 0, ErrOutOfRange
	}

	// 经度直接计算
	lon = math.Atan2(y, x) * 180 / math.Pi

	// 纬度迭代计算
	p := math.Sqrt(x*x + y*y)
	theta := math.Atan2(z*wgs84A, p*wgs84B)

	// 迭代求解纬度（通常 3-5 次收敛）
	lat = theta
	for i := 0; i < 10; i++ {
		latPrev := lat
		lat = math.Atan2(z+wgs84Ep2*wgs84B*math.Pow(math.Sin(theta), 3),
			p-wgs84E2*wgs84A*math.Pow(math.Cos(theta), 3))
		theta = lat
		if math.Abs(lat-latPrev) < 1e-12 {
			break
		}
	}
	lat = lat * 180 / math.Pi

	// 高度计算
	sinLat := math.Sin(lat * math.Pi / 180)
	cosLat := math.Cos(lat * math.Pi / 180)
	N := wgs84A / math.Sqrt(1-wgs84E2*sinLat*sinLat)
	if cosLat > 1e-10 {
		h = p/cosLat - N
	} else {
		h = z/sinLat - N*(1-wgs84E2)
	}

	return lat, lon, h, nil
}

// DistanceBetweenPositions 计算两个 AntennaPosition 之间的距离（米）。
// 使用 Haversine 公式计算球面距离。
func DistanceBetweenPositions(p1, p2 *AntennaPosition) float64 {
	if p1 == nil || p2 == nil {
		return 0
	}

	const R = 6371000.0 // 地球平均半径 (m)

	dLat := (p2.Latitude - p1.Latitude) * math.Pi / 180
	dLon := (p2.Longitude - p1.Longitude) * math.Pi / 180

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)

	a := sinDLat*sinDLat + math.Cos(p1.Latitude*math.Pi/180)*math.Cos(p2.Latitude*math.Pi/180)*sinDLon*sinDLon
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// Decode1005 解析 RTCM 1005 报文，提取基站天线位置。
func Decode1005(pkt *RTCMPacket) (*AntennaPosition, error) {
	data := pkt.Data

	// 最小帧长度检查
	// 1005 payload 至少 125 bits ≈ 16 bytes
	if len(data) < 22 {
		return nil, fmt.Errorf("frame too short for 1005: %d bytes", len(data))
	}

	// 验证消息类型
	msgType := ExtractMsgType(pkt)
	if msgType != 1005 {
		return nil, fmt.Errorf("message type %d is not 1005", msgType)
	}

	// 提取 payload
	payloadLen := int(data[1]&0x03)<<8 | int(data[2])
	// 1005 完整报文 payload 为 152 bits ≈ 19 bytes
	// 最少需要 19 bytes payload 才能包含完整的 ECEF XYZ 坐标
	if payloadLen < 19 {
		return nil, fmt.Errorf("payload too short for 1005: %d bytes (need 19)", payloadLen)
	}
	payload := data[3 : 3+payloadLen]

	// 使用 bitReader 解析
	br := &bitReader{data: payload}

	// 跳过已解析的字段
	br.skip(12) // DF002 - 消息类型
	br.skip(12) // DF003 - 站点 ID
	br.skip(6)  // DF021 - ITRF 年份
	br.skip(4)  // DF022, DF141, DF142, DF001 (GPS, GLONASS, Galileo, Ref Station Indicator)

	// 读取 ECEF 坐标（每个坐标后有 2 bits Orbit Eph Flag）
	x := br.readSigned38()
	br.skip(2) // Orbit Eph Flag for X (实际是 2 bits)
	y := br.readSigned38()
	br.skip(2) // Orbit Eph Flag for Y (实际是 2 bits)
	z := br.readSigned38()

	if br.err != nil {
		return nil, fmt.Errorf("bit field extraction failed: %w", br.err)
	}

	// 单位转换: 0.0001m → m
	xMeter := float64(x) * 0.0001
	yMeter := float64(y) * 0.0001
	zMeter := float64(z) * 0.0001

	// 转换为经纬度
	lat, lon, h, err := ecefToLatLng(xMeter, yMeter, zMeter)
	if err != nil {
		return nil, fmt.Errorf("coordinate conversion failed: %w", err)
	}

	return &AntennaPosition{
		Latitude:  lat,
		Longitude: lon,
		Height:    h,
		UpdatedAt: time.Now().Unix(),
	}, nil
}

// bitReader 辅助解析 RTCM bit fields
type bitReader struct {
	data   []byte
	offset int
	err    error
}

func (br *bitReader) skip(n int) {
	br.offset += n
}

func (br *bitReader) readUint(n int) uint64 {
	if br.err != nil {
		return 0
	}

	var result uint64
	for i := 0; i < n; i++ {
		byteIndex := br.offset / 8
		bitIndex := 7 - (br.offset % 8) // MSB-first: bit 7 is highest

		if byteIndex >= len(br.data) {
			br.err = fmt.Errorf("insufficient data at bit %d", br.offset)
			return 0
		}

		bit := (br.data[byteIndex] >> bitIndex) & 1
		result = (result << 1) | uint64(bit)
		br.offset++
	}
	return result
}

func (br *bitReader) readSigned38() int64 {
	u := br.readUint(38)
	if br.err != nil {
		return 0
	}
	if u >= (1 << 37) {
		return int64(u) - (1 << 38)
	}
	return int64(u)
}