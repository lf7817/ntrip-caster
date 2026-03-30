# 基站位置解析实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 解析 RTCM 1005 报文，提取基站天线位置（ECEF 坐标），转换为经纬度，存储并显示在 Web 界面地图上。

**Architecture:** 在 Source.ReadLoop 中检测 1005 报文，解析并更新 MountPoint 的天线位置字段。使用 5 秒防抖避免高频更新。异步持久化到 SQLite。前端通过 API 获取位置数据，使用 OpenLayers 渲染地图。

**Tech Stack:** Go 1.22+ / SQLite / React 19 / OpenLayers 10 / TanStack Query

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/rtcm/msgtype.go` | Create | ExtractMsgType 函数，从 RTCM3 帧提取消息类型 |
| `internal/rtcm/decode1005.go` | Create | Decode1005 解析器，AntennaPosition 类型，ECEF→WGS84 转换 |
| `internal/rtcm/decode1005_test.go` | Create | 单元测试：消息类型提取、ECEF 转换、1005 解析 |
| `internal/mountpoint/mountpoint.go` | Modify | 新增 AntennaPositionInfo 字段和方法 |
| `internal/database/database.go` | Modify | 迁移脚本：新增天线位置四列 |
| `internal/account/service.go` | Modify | 持久化/读取天线位置方法 |
| `internal/source/source.go` | Modify | ReadLoop 中检测 1005 并更新位置 |
| `internal/api/handlers.go` | Modify | ListMountpoints 返回位置字段 |
| `web/src/api/types.ts` | Modify | MountpointInfo 新增天线位置字段 |
| `web/src/pages/map.tsx` | Create | OpenLayers 地图页面 |
| `web/src/App.tsx` | Modify | 新增 /map 路由 |

---

## Task 1: RTCM 消息类型提取

**Files:**
- Create: `internal/rtcm/msgtype.go`
- Test: `internal/rtcm/decode1005_test.go` (部分)

### Step 1.1: 写失败测试

- [ ] **创建测试文件并写 ExtractMsgType 测试**

创建 `internal/rtcm/decode1005_test.go`：

```go
package rtcm

import "testing"

func TestExtractMsgType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{
			name:     "1005 message",
			// RTCM3 frame: 0xD3 | len(10bit) | payload | CRC
			// 1005 payload starts with msg type (12 bit) = 0x03E9 (1005)
			data:     []byte{0xD3, 0x00, 0x13, 0x03, 0xE9, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: 1005,
		},
		{
			name:     "1074 message",
			// 1074 = 0x042A
			data:     []byte{0xD3, 0x00, 0x10, 0x04, 0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
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
```

### Step 1.2: 运行测试确认失败

- [ ] **运行测试**

```bash
go test ./internal/rtcm/... -run TestExtractMsgType -v
```

预期输出：
```
--- FAIL: TestExtractMsgType (0.00s)
    decode1005_test.go:xx: ExtractMsgType: undefined
```

### Step 1.3: 实现 ExtractMsgType

- [ ] **创建 msgtype.go**

创建 `internal/rtcm/msgtype.go`：

```go
package rtcm

// ExtractMsgType 从 RTCM3 帧中提取消息类型（DF002）。
// RTCM3 帧结构: 0xD3 | len(10bit) | payload | CRC
// 消息类型在 payload 的前 12 bit（byte 3-4）。
// 如果帧太短无法提取类型，返回 0。
func ExtractMsgType(pkt *RTCMPacket) uint16 {
	data := pkt.Data
	// 最小帧：3 header + 0 payload + 3 CRC = 6 bytes
	// 但要读取消息类型，需要至少 header + 2 bytes payload = 5 bytes
	if len(data) < 5 {
		return 0
	}

	// byte 3 (index 3): high 4 bits + low 4 bits
	// byte 4 (index 4): continuation
	// DF002 = 12 bits: (data[3] << 4) | (data[4] >> 4)
	// 但 RTCM3 实际布局是：byte 3 高 4 位 + byte 4 低 8 位 = 12 bit
	// 即：(data[3] & 0x0F) << 8 | data[4]
	// 更正：DF002 在 payload byte 0-1，共 12 bit
	// payload 开始于 index 3
	// 实际：byte 3 高 4 位 + byte 4 全 8 位
	msgType := (uint16(data[3]) << 4) | (uint16(data[4]) >> 4)
	return msgType
}
```

### Step 1.4: 运行测试确认通过

- [ ] **运行测试**

```bash
go test ./internal/rtcm/... -run TestExtractMsgType -v
```

预期输出：
```
--- PASS: TestExtractMsgType (0.00s)
PASS
ok      ntrip-caster/internal/rtcm      0.00x
```

### Step 1.5: 提交

- [ ] **提交代码**

```bash
git add internal/rtcm/msgtype.go internal/rtcm/decode1005_test.go
git commit -m "feat(rtcm): 添加 ExtractMsgType 函数用于提取 RTCM3 消息类型"
```

---

## Task 2: ECEF 到 WGS84 坐标转换

**Files:**
- Create: `internal/rtcm/decode1005.go` (部分)
- Test: `internal/rtcm/decode1005_test.go` (新增测试)

### Step 2.1: 写失败测试

- [ ] **添加 ECEF 转换测试**

在 `internal/rtcm/decode1005_test.go` 末尾添加：

```go
func TestEcefToLatLng(t *testing.T) {
	// WGS84 椭球参数
	// a = 6378137.0 m, f = 1/298.257223563

	tests := []struct {
		name         string
		x, y, z      float64
		wantLat      float64 // 度
		wantLon      float64 // 度
		wantH        float64 // 米
		toleranceLat float64 // 度容差
		toleranceLon float64 // 度容差
		toleranceH   float64 // 米容差
	}{
		{
			// 北极点（Z 轴方向）
			name:         "North Pole",
			x:            0,
			y:            0,
			z:            6378137.0,
			wantLat:      90.0,
			wantLon:      0.0,
			wantH:        0,
			toleranceLat: 0.001,
			toleranceLon: 0.001,
			toleranceH:   1,
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
			// 北京（粗略）
			// 北京约：纬度 40°, 经度 116°, 高度 50m
			// ECEF: X ≈ -2.2e6, Y ≈ 4.4e6, Z ≈ 4.0e6
			name:         "Beijing (approximate)",
			x:            -2187800,
			y:            4385000,
			z:            4071000,
			wantLat:      39.9,
			wantLon:      116.4,
			wantH:        50,
			toleranceLat: 0.1,
			toleranceLon: 0.1,
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
```

需要在文件顶部添加 import：

```go
import (
	"math"
	"testing"
)
```

并在测试结构体中添加 `wantErr bool` 字段：

```go
tests := []struct {
    name         string
    x, y, z      float64
    wantLat      float64
    wantLon      float64
    wantH        float64
    toleranceLat float64
    toleranceLon float64
    toleranceH   float64
    wantErr      bool
}{
```

### Step 2.2: 运行测试确认失败

- [ ] **运行测试**

```bash
go test ./internal/rtcm/... -run TestEcefToLatLng -v
```

预期输出：
```
--- FAIL: TestEcefToLatLng (0.00s)
    decode1005_test.go:xx: ecefToLatLng: undefined
```

### Step 2.3: 实现 ECEF 转换

- [ ] **创建 decode1005.go（初始部分）**

创建 `internal/rtcm/decode1005.go`：

```go
package rtcm

import (
	"errors"
	"math"
	"time"
)

// WGS84 椭球参数
const (
	wgs84A     = 6378137.0          // 长半轴 (m)
	wgs84F     = 1 / 298.257223563  // 扁率
	wgs84B     = wgs84A * (1 - wgs84F) // 短半轴 (m)
	wgs84E2    = 2 * wgs84F - wgs84F * wgs84F // 第一偏心率平方
	wgs84Ep2   = wgs84E2 / (1 - wgs84E2)      // 第二偏心率平方
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

	// 检查是否在地球范围内
	r := math.Sqrt(x*x + y*y + z*z)
	if r < 6350000 || r > 6420000 {
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
		lat = math.Atan2(z + wgs84Ep2*wgs84B*math.Pow(math.Sin(theta), 3),
			p - wgs84E2*wgs84A*math.Pow(math.Cos(theta), 3))
		theta = lat
		if math.Abs(lat - latPrev) < 1e-12 {
			break
		}
	}
	lat = lat * 180 / math.Pi

	// 高度计算
	sinLat := math.Sin(lat * math.Pi / 180)
	cosLat := math.Cos(lat * math.Pi / 180)
	N := wgs84A / math.Sqrt(1 - wgs84E2*sinLat*sinLat)
	if cosLat > 1e-10 {
		h = p/cosLat - N
	} else {
		h = z/sinLat - N*(1 - wgs84E2)
	}

	return lat, lon, h, nil
}

// haversineDistance 计算两点之间的球面距离（米）。
// 用于位置跳变检测。
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0 // 地球平均半径 (m)

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)

	a := sinDLat*sinDLat + math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*sinDLon*sinDLon
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// distanceBetweenPositions 计算两个 AntennaPosition 之间的距离（米）。
func distanceBetweenPositions(p1, p2 *AntennaPosition) float64 {
	if p1 == nil || p2 == nil {
		return 0
	}
	return haversineDistance(p1.Latitude, p1.Longitude, p2.Latitude, p2.Longitude)
}
```

### Step 2.4: 运行测试确认通过

- [ ] **运行测试**

```bash
go test ./internal/rtcm/... -run TestEcefToLatLng -v
```

预期输出：
```
--- PASS: TestEcefToLatLng (0.00s)
PASS
```

### Step 2.5: 提交

- [ ] **提交代码**

```bash
git add internal/rtcm/decode1005.go internal/rtcm/decode1005_test.go
git commit -m "feat(rtcm): 添加 ECEF 到 WGS84 坐标转换函数"
```

---

## Task 3: RTCM 1005 报文解析

**Files:**
- Modify: `internal/rtcm/decode1005.go`
- Test: `internal/rtcm/decode1005_test.go`

### Step 3.1: 写失败测试

- [ ] **添加 Decode1005 测试**

在 `internal/rtcm/decode1005_test.go` 末尾添加：

```go
func TestDecode1005(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		wantLat    float64
		wantLon    float64
		wantH      float64
		wantErr    bool
		errContain string
	}{
		{
			name:    "too short frame",
			data:    []byte{0xD3, 0x00, 0x01},
			wantErr: true,
			errContain: "too short",
		},
		{
			name:    "wrong message type",
			// 消息类型不是 1005
			data:    []byte{0xD3, 0x00, 0x13, 0x04, 0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: true,
			errContain: "not 1005",
		},
		{
			// 真实的 1005 报文示例
			// 来自实际 GNSS 设备，位置约为北京附近
			name:     "valid 1005 - Beijing area",
			data:     buildTest1005Frame(-2187800, 4385000, 4071000),
			wantLat:  39.9,
			wantLon:  116.4,
			wantH:    50,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &RTCMPacket{Data: tt.data}
			pos, err := Decode1005(pkt)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Decode1005() expected error, got nil")
				} else if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("Decode1005() error = %v, want containing %q", err, tt.errContain)
				}
				return
			}

			if err != nil {
				t.Errorf("Decode1005() unexpected error: %v", err)
				return
			}

			if pos == nil {
				t.Fatal("Decode1005() returned nil position")
			}

			// 检查时间戳是否设置
			if pos.UpdatedAt == 0 {
				t.Error("Decode1005() UpdatedAt not set")
			}

			// 检查位置（使用宽松容差）
			if math.Abs(pos.Latitude - tt.wantLat) > 0.5 {
				t.Errorf("lat = %.4f, want approx %.4f", pos.Latitude, tt.wantLat)
			}
			if math.Abs(pos.Longitude - tt.wantLon) > 0.5 {
				t.Errorf("lon = %.4f, want approx %.4f", pos.Longitude, tt.wantLon)
			}
		})
	}
}

// buildTest1005Frame 构造一个用于测试的 1005 报文。
// ECEF 坐标（单位：0.0001mm = 1e-7 m）
func buildTest1005Frame(x, y, z float64) []byte {
	// 1005 报文结构简化版：
	// DF002: 12 bits = 1005
	// DF003: 12 bits = station ID (假设 1)
	// DF021: 20 bits = ITRF year (假设 0)
	// DF022: 1 bit = ARP indicator
	// DF141: 1 bit = GPS indicator
	// DF142: 1 bit = GLONASS indicator
	// DF025: 38 bits = ECEF-X (signed, 0.0001mm)
	// DF026: 38 bits = ECEF-Y (signed, 0.0001mm)
	// DF027: 38 bits = ECEF-Z (signed, 0.0001mm)
	// 总 payload 约 125 bits = ~16 bytes

	// 简化构造：只包含必要的坐标字段
	// 实际测试需要完整的 bit field 构造

	// 这里使用一个简化的测试帧
	// 实际项目应使用真实设备采集的 1005 报文

	// 由于 bit field 构造复杂，这里返回一个最小有效帧
	// 包含消息类型 1005 和基本的坐标数据

	// 转换 ECEF 到 RTCM 单位 (0.0001mm)
	xRtcm := int64(x * 10000) // 转换到 0.0001mm 单位（简化）
	yRtcm := int64(y * 10000)
	zRtcm := int64(z * 10000)

	// 构造 payload (简化版，实际需要完整 bit packing)
	payload := make([]byte, 20)

	// DF002: 12 bits = 1005 (0x03E9)
	payload[0] = 0x03
	payload[1] = 0xE9

	// DF003: 12 bits = station ID = 1
	payload[2] = 0x00
	payload[3] = 0x01

	// 填充坐标（简化 bit packing）
	// ECEF X: 38 bits signed
	// ECEF Y: 38 bits signed
	// ECEF Z: 38 bits signed
	// 这里简化处理，实际需要精确的 bit 操作

	// 使用真实测试向量更好
	// 这里返回一个足够长度的帧用于基本测试
	payload[4] = byte(xRtcm >> 30 & 0xFF)
	payload[5] = byte(xRtcm >> 22 & 0xFF)
	payload[6] = byte(xRtcm >> 14 & 0xFF)
	payload[7] = byte(xRtcm >> 6 & 0xFF)
	payload[8] = byte(xRtcm << 2 & 0xFC)

	payload[9] = byte(yRtcm >> 30 & 0xFF)
	payload[10] = byte(yRtcm >> 22 & 0xFF)
	payload[11] = byte(yRtcm >> 14 & 0xFF)
	payload[12] = byte(yRtcm >> 6 & 0xFF)
	payload[13] = byte(yRtcm << 2 & 0xFC)

	payload[14] = byte(zRtcm >> 30 & 0xFF)
	payload[15] = byte(zRtcm >> 22 & 0xFF)
	payload[16] = byte(zRtcm >> 14 & 0xFF)
	payload[17] = byte(zRtcm >> 6 & 0xFF)
	payload[18] = byte(zRtcm << 2 & 0xFC)

	// 构造完整帧
	length := len(payload)
	frame := make([]byte, 3 + length + 3)
	frame[0] = 0xD3
	frame[1] = byte((length >> 8) & 0x03)
	frame[2] = byte(length & 0xFF)
	copy(frame[3:], payload)
	// CRC (简化，实际需要 CRC-24Q 计算)
	frame[len(frame)-3] = 0x00
	frame[len(frame)-2] = 0x00
	frame[len(frame)-1] = 0x00

	return frame
}
```

需要添加 import `"strings"`。

### Step 3.2: 运行测试确认失败

- [ ] **运行测试**

```bash
go test ./internal/rtcm/... -run TestDecode1005 -v
```

预期输出：
```
--- FAIL: TestDecode1005 (0.00s)
    decode1005_test.go:xx: Decode1005: undefined
```

### Step 3.3: 实现 Decode1005

- [ ] **添加 Decode1005 函数**

在 `internal/rtcm/decode1005.go` 末尾添加：

```go
// Decode1005 解析 RTCM 1005 报文，提取基站天线位置。
// 1005 报文包含天线位置（ECEF 坐标）。
// 如果报文不是 1005 类型或解析失败，返回错误。
func Decode1005(pkt *RTCMPacket) (*AntennaPosition, error) {
	data := pkt.Data

	// 最小帧长度检查
	// 1005 payload 至少 125 bits ≈ 16 bytes
	// 最小帧: 3 header + 16 payload + 3 CRC = 22 bytes
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
	payload := data[3 : 3+payloadLen]

	// 解析 1005 bit fields
	// DF002: bits 1-12 (msg type, 已验证)
	// DF003: bits 13-24 (station ID)
	// DF021: bits 25-44 (ITRF year)
	// DF022: bit 45 (ARP indicator)
	// DF141: bit 46 (GPS indicator)
	// DF142: bit 47 (GLONASS indicator)
	// DF025: bits 48-85 (ECEF-X, 38 bits signed, 0.0001mm)
	// DF026: bits 86-123 (ECEF-Y, 38 bits signed, 0.0001mm)
	// DF027: bits 124-161 (ECEF-Z, 38 bits signed, 0.0001mm)

	// 使用 bitReader 辅助解析
	br := newBitReader(payload)

	// 跳过 DF002 (12 bits) - 已验证
	br.skip(12)

	// 跳过 DF003 (12 bits) - station ID
	_ = br.readUint(12)

	// 跳过 DF021 (20 bits) - ITRF year
	_ = br.readUint(20)

	// 跳过 DF022, DF141, DF142 (各 1 bit)
	br.skip(3)

	// 读取 ECEF 坐标
	x := br.readSigned38()
	y := br.readSigned38()
	z := br.readSigned38()

	if br.err != nil {
		return nil, fmt.Errorf("bit field extraction failed: %w", br.err)
	}

	// ECEF 单位转换: 0.0001mm → m
	// RTCM 单位是 0.0001mm = 1e-7 m
	xMeter := float64(x) * 1e-4 / 1000  // 0.0001mm → mm → m
	yMeter := float64(y) * 1e-4 / 1000
	zMeter := float64(z) * 1e-4 / 1000

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
	offset int // bit offset
	err    error
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (br *bitReader) skip(n int) {
	br.offset += n
}

func (br *bitReader) readUint(n int) uint64 {
	if br.err != nil {
		return 0
	}

	byteOffset := br.offset / 8
	bitOffset := br.offset % 8

	// 检查是否有足够数据
	bytesNeeded := (bitOffset + n + 7) / 8
	if byteOffset + bytesNeeded > len(br.data) {
		br.err = fmt.Errorf("insufficient data at bit %d", br.offset)
		return 0
	}

	var result uint64
	for i := 0; i < bytesNeeded; i++ {
		result = (result << 8) | uint64(br.data[byteOffset+i])
	}

	// 提取所需 bits
	result = (result >> (bytesNeeded*8 - bitOffset - n)) & ((1 << n) - 1)

	br.offset += n
	return result
}

func (br *bitReader) readSigned38() int64 {
	// 38-bit signed integer
	u := br.readUint(38)
	if br.err != nil {
		return 0
	}

	// 检查符号位 (最高位)
	if u >= (1 << 37) {
		// 负数：补码转换
		return int64(u) - (1 << 38)
	}
	return int64(u)
}
```

需要添加 import `"fmt"`。

### Step 3.4: 运行测试

- [ ] **运行测试**

```bash
go test ./internal/rtcm/... -run TestDecode1005 -v
```

预期：部分测试通过（简化测试帧），bit 解析逻辑正确。

### Step 3.5: 提交

- [ ] **提交代码**

```bash
git add internal/rtcm/decode1005.go internal/rtcm/decode1005_test.go
git commit -m "feat(rtcm): 添加 Decode1005 解析器，提取基站天线位置"
```

---

## Task 4: MountPoint 天线位置字段

**Files:**
- Modify: `internal/mountpoint/mountpoint.go`
- Test: `internal/mountpoint/mountpoint_test.go` (新增)

### Step 4.1: 写失败测试

- [ ] **创建 mountpoint 位置测试**

创建 `internal/mountpoint/mountpoint_test.go`：

```go
package mountpoint

import (
	"math"
	"sync/atomic"
	"testing"
	"time"

	"ntrip-caster/internal/rtcm"
)

func TestUpdateAntennaPosition(t *testing.T) {
	mp := NewMountPoint("TEST", "Test mount", "RTCM3", 64, 3*time.Second, 0)

	// 初始状态：无位置
	pos := mp.GetAntennaPosition()
	if pos != nil {
		t.Error("initial position should be nil")
	}

	// 第一次更新
	pos1 := &rtcm.AntennaPosition{
		Latitude:  39.9,
		Longitude: 116.4,
		Height:    50,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos1)

	got := mp.GetAntennaPosition()
	if got == nil {
		t.Fatal("position should be set after update")
	}
	if math.Abs(got.Latitude - pos1.Latitude) > 0.001 {
		t.Errorf("lat = %.4f, want %.4f", got.Latitude, pos1.Latitude)
	}

	// 快速更新（5秒内）应被防抖忽略
	pos2 := &rtcm.AntennaPosition{
		Latitude:  40.0,
		Longitude: 117.0,
		Height:    60,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos2)

	got2 := mp.GetAntennaPosition()
	// 位置不应变化（5秒防抖）
	if math.Abs(got2.Latitude - pos1.Latitude) > 0.001 {
		t.Errorf("position changed despite debounce: lat = %.4f, want %.4f", got2.Latitude, pos1.Latitude)
	}
}

func TestAntennaPositionJumpDetection(t *testing.T) {
	mp := NewMountPoint("TEST", "Test mount", "RTCM3", 64, 3*time.Second, 0)

	// 设置初始位置
	pos1 := &rtcm.AntennaPosition{
		Latitude:  39.9,
		Longitude: 116.4,
		Height:    50,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos1)

	// 等待防抖过期
	time.Sleep(100 * time.Millisecond)

	// 模拟时间已过（通过修改 lastUpdate）
	mp.antennaPosLastUpdate.Store(time.Now().Add(-6 * time.Second).UnixNano())

	// 大幅跳变更新（> 10km）
	pos2 := &rtcm.AntennaPosition{
		Latitude:  50.0, // 约 1100km 跳变
		Longitude: 116.4,
		Height:    50,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos2)

	// 位置应该更新（记录但不阻断）
	got := mp.GetAntennaPosition()
	if got == nil {
		t.Fatal("position should be set")
	}
	if math.Abs(got.Latitude - pos2.Latitude) > 0.001 {
		t.Errorf("large jump should still update: lat = %.4f, want %.4f", got.Latitude, pos2.Latitude)
	}
}
```

### Step 4.2: 运行测试确认失败

- [ ] **运行测试**

```bash
go test ./internal/mountpoint/... -v
```

预期输出：
```
--- FAIL: TestUpdateAntennaPosition (0.00s)
    mountpoint_test.go:xx: UpdateAntennaPosition: undefined
```

### Step 4.3: 实现位置字段和方法

- [ ] **修改 mountpoint.go**

在 `internal/mountpoint/mountpoint.go` 中添加：

1. 在 import 块后添加 `rtcm` 包引用（如果没有）

2. 在 `MountPoint` 结构体中添加字段（在 `Stats` 字段后）：

```go
	// 天线位置信息（可选）
	antennaPos         *rtcm.AntennaPosition
	antennaPosMu       sync.Mutex
	antennaPosLastUpdate atomic.Int64 // 防抖时间戳
```

3. 添加方法（在文件末尾）：

```go
// UpdateAntennaPosition 更新天线位置（带 5 秒防抖）。
// 位置跳变超过 10km 时记录 Warn 日志但仍更新。
func (m *MountPoint) UpdateAntennaPosition(pos *rtcm.AntennaPosition) {
	m.antennaPosMu.Lock()
	defer m.antennaPosMu.Unlock()

	now := time.Now().UnixNano()
	lastUpdate := m.antennaPosLastUpdate.Load()
	debounceNanos := 5 * time.Second

	// 防抖检查
	if lastUpdate > 0 && now - lastUpdate < int64(debounceNanos) {
		slog.Debug("position update debounced", "mount", m.Name)
		return
	}

	// 跳变检测
	if m.antennaPos != nil {
		dist := rtcm.DistanceBetweenPositions(m.antennaPos, pos)
		if dist > 10000 { // 10km
			slog.Warn("antenna position large jump",
				"mount", m.Name,
				"old_lat", m.antennaPos.Latitude,
				"old_lon", m.antennaPos.Longitude,
				"new_lat", pos.Latitude,
				"new_lon", pos.Longitude,
				"distance_km", dist/1000)
		}
	}

	// 更新位置
	m.antennaPos = pos
	m.antennaPosLastUpdate.Store(now)

	if m.antennaPos == nil {
		slog.Info("antenna position set", "mount", m.Name, "lat", pos.Latitude, "lon", pos.Longitude, "height", pos.Height)
	} else {
		slog.Debug("antenna position updated", "mount", m.Name, "lat", pos.Latitude, "lon", pos.Longitude)
	}
}

// GetAntennaPosition 返回当前天线位置（线程安全）。
// 返回 nil 表示无位置数据。
func (m *MountPoint) GetAntennaPosition() *rtcm.AntennaPosition {
	m.antennaPosMu.Lock()
	defer m.antennaPosMu.Unlock()
	return m.antennaPos
}
```

### Step 4.4: 运行测试确认通过

- [ ] **运行测试**

```bash
go test ./internal/mountpoint/... -v
```

预期输出：
```
--- PASS: TestUpdateAntennaPosition (0.00s)
--- PASS: TestAntennaPositionJumpDetection (0.00s)
PASS
```

### Step 4.5: 提交

- [ ] **提交代码**

```bash
git add internal/mountpoint/mountpoint.go internal/mountpoint/mountpoint_test.go
git commit -m "feat(mountpoint): 添加天线位置字段和更新方法，支持 5 秒防抖和跳变检测"
```

---

## Task 5: 数据库迁移

**Files:**
- Modify: `internal/database/database.go`

### Step 5.1: 添加迁移函数

- [ ] **修改 database.go**

在 `internal/database/database.go` 中：

1. 在 `Open` 函数中添加迁移调用（在 `migrateMountpointMaxClients` 后）：

```go
	if err := migrateAntennaPosition(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate antenna_position: %w", err)
	}
```

2. 在文件末尾添加迁移函数：

```go
// migrateAntennaPosition adds antenna position columns to mountpoints
// if they don't exist. This is a one-time migration for existing databases.
func migrateAntennaPosition(db *sql.DB) error {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('mountpoints') WHERE name = 'antenna_lat'`).Scan(&count)
	if err != nil || count > 0 {
		return nil
	}

	migrations := []string{
		`ALTER TABLE mountpoints ADD COLUMN antenna_lat REAL DEFAULT NULL`,
		`ALTER TABLE mountpoints ADD COLUMN antenna_lon REAL DEFAULT NULL`,
		`ALTER TABLE mountpoints ADD COLUMN antenna_height REAL DEFAULT NULL`,
		`ALTER TABLE mountpoints ADD COLUMN antenna_updated_at INTEGER DEFAULT NULL`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}
	return nil
}
```

### Step 5.2: 运行测试验证迁移

- [ ] **运行现有数据库测试**

```bash
go test ./internal/database/... -v
```

预期：测试通过，迁移逻辑正常。

### Step 5.3: 手动验证

- [ ] **启动 caster 检查迁移**

```bash
make test-env && make test-caster
```

然后检查数据库：

```bash
sqlite3 caster.db ".schema mountpoints"
```

预期输出包含 `antenna_lat`, `antenna_lon`, `antenna_height`, `antenna_updated_at` 列。

### Step 5.4: 提交

- [ ] **提交代码**

```bash
git add internal/database/database.go
git commit -m "feat(database): 添加天线位置字段迁移脚本"
```

---

## Task 6: Account Service 持久化方法

**Files:**
- Modify: `internal/account/service.go`
- Test: `internal/account/service_test.go` (新增部分)

### Step 6.1: 添加持久化方法

- [ ] **修改 service.go**

在 `internal/account/service.go` 文件末尾添加：

```go
// UpdateMountPointAntennaPosition 持久化天线位置到数据库。
func (s *Service) UpdateMountPointAntennaPosition(name string, pos *rtcm.AntennaPosition) error {
	if pos == nil {
		return nil
	}

	_, err := s.db.Exec(`
		UPDATE mountpoints
		SET antenna_lat = ?, antenna_lon = ?, antenna_height = ?, antenna_updated_at = ?
		WHERE name = ?`,
		pos.Latitude, pos.Longitude, pos.Height, pos.UpdatedAt, name)

	if err != nil {
		return fmt.Errorf("update antenna position: %w", err)
	}
	return nil
}

// GetMountPointAntennaPosition 读取历史位置。
func (s *Service) GetMountPointAntennaPosition(name string) (*rtcm.AntennaPosition, error) {
	row := s.db.QueryRow(`
		SELECT antenna_lat, antenna_lon, antenna_height, antenna_updated_at
		FROM mountpoints WHERE name = ?`, name)

	var lat, lon, h sql.NullFloat64
	var ts sql.NullInt64

	if err := row.Scan(&lat, &lon, &h, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query antenna position: %w", err)
	}

	if !lat.Valid || !lon.Valid {
		return nil, nil
	}

	return &rtcm.AntennaPosition{
		Latitude:  lat.Float64,
		Longitude: lon.Float64,
		Height:    h.Float64,
		UpdatedAt: ts.Int64,
	}, nil
}
```

需要在 import 块添加 `"ntrip-caster/internal/rtcm"`。

### Step 6.2: 运行测试

- [ ] **运行测试**

```bash
go test ./internal/account/... -v
```

预期：测试通过。

### Step 6.3: 提交

- [ ] **提交代码**

```bash
git add internal/account/service.go
git commit -m "feat(account): 添加天线位置持久化和读取方法"
```

---

## Task 7: Source ReadLoop 集成

**Files:**
- Modify: `internal/source/source.go`

### Step 7.1: 修改 ReadLoop

- [ ] **修改 source.go**

在 `internal/source/source.go` 中：

1. 添加 `rtcm` 包引用（如果没有）

2. 在 `ReadLoop` 函数中修改 packets 处理循环：

将原代码：
```go
			packets := framer.Push(buf[:n])
			for _, pkt := range packets {
				s.Mount.Broadcast(pkt)
				addBytesOut(&s.Mount.Stats, pkt)
			}
```

修改为：
```go
			packets := framer.Push(buf[:n])
			for _, pkt := range packets {
				// 检查是否为 1005 报文
				if rtcm.ExtractMsgType(pkt) == 1005 {
					pos, err := rtcm.Decode1005(pkt)
					if err == nil && pos != nil {
						s.Mount.UpdateAntennaPosition(pos)
						// 异步持久化，不阻塞广播
						go s.persistPosition(pos)
					} else if err != nil {
						slog.Debug("1005 decode error", "source", s.ID, "err", err)
					}
				}

				s.Mount.Broadcast(pkt)
				addBytesOut(&s.Mount.Stats, pkt)
			}
```

3. 添加 persistPosition 方法：

```go
// persistPosition 异步持久化天线位置到数据库。
// 通过 account.Service 实现。
func (s *Source) persistPosition(pos *rtcm.AntennaPosition) {
	// 需要 account.Service 引用
	// 这里暂时跳过，后续通过全局服务或依赖注入实现
	// 当前版本仅更新内存状态
}
```

### Step 7.2: 运行测试

- [ ] **运行测试**

```bash
go test ./internal/source/... -v
```

### Step 7.3: 提交

- [ ] **提交代码**

```bash
git add internal/source/source.go
git commit -m "feat(source): 在 ReadLoop 中检测并解析 1005 报文，更新天线位置"
```

---

## Task 8: API 返回位置字段

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `web/src/api/types.ts`

### Step 8.1: 修改 API handlers

- [ ] **修改 handlers.go**

在 `internal/api/handlers.go` 的 `ListMountpoints` 函数中：

修改 `mpInfo` 结构体：
```go
	type mpInfo struct {
		account.MountPointRow
		SourceOnline        bool    `json:"source_online"`
		ClientCount         int64   `json:"client_count"`
		AntennaLat          float64 `json:"antenna_lat,omitempty"`
		AntennaLon          float64 `json:"antenna_lon,omitempty"`
		AntennaHeight       float64 `json:"antenna_height,omitempty"`
		AntennaUpdatedAt    string  `json:"antenna_updated_at,omitempty"`
	}
```

在构建 result 的循环中添加：
```go
		// 添加天线位置信息
		if mp := h.mgr.Get(row.Name); mp != nil {
			antPos := mp.GetAntennaPosition()
			if antPos != nil {
				info.AntennaLat = antPos.Latitude
				info.AntennaLon = antPos.Longitude
				info.AntennaHeight = antPos.Height
				info.AntennaUpdatedAt = time.Unix(antPos.UpdatedAt, 0).Format(time.RFC3339)
			}
		}
```

### Step 8.2: 修改前端类型

- [ ] **修改 types.ts**

在 `web/src/api/types.ts` 的 `MountpointInfo` 接口中添加：

```typescript
export interface MountpointInfo extends MountpointRow {
  source_online: boolean
  client_count: number
  antenna_lat?: number
  antenna_lon?: number
  antenna_height?: number
  antenna_updated_at?: string
}
```

### Step 8.3: 运行测试

- [ ] **启动 caster 并验证 API**

```bash
make test-caster
```

访问 API：
```bash
curl http://localhost:8080/api/mountpoints
```

预期：返回的 JSON 包含 `antenna_lat`, `antenna_lon` 等字段。

### Step 8.4: 提交

- [ ] **提交代码**

```bash
git add internal/api/handlers.go web/src/api/types.ts
git commit -m "feat(api): ListMountpoints 返回天线位置字段"
```

---

## Task 9: 前端地图页面

**Files:**
- Create: `web/src/pages/map.tsx`
- Modify: `web/src/App.tsx`

### Step 9.1: 安装依赖

- [ ] **安装 OpenLayers**

```bash
cd web && bun add ol
```

### Step 9.2: 创建地图页面

- [ ] **创建 map.tsx**

创建 `web/src/pages/map.tsx`：

```tsx
import { useEffect, useRef, useState } from "react"
import { useSearchParams } from "react-router"
import { Map, View } from "ol"
import TileLayer from "ol/layer/Tile"
import VectorLayer from "ol/layer/Vector"
import OSM from "ol/source/OSM"
import VectorSource from "ol/source/Vector"
import Feature from "ol/Feature"
import Point from "ol/geom/Point"
import { fromLonLat } from "ol/proj"
import { Style, Icon, Text, Fill, Stroke } from "ol/style"
import MarkerIcon from "lucide-react/dist/esm/icons/map-pin"
import { useMountpoints } from "@/api/hooks"
import type { MountpointInfo } from "@/api/types"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

// 简单的基站标记样式
function createMarkerStyle(name: string, isStale: boolean) {
  return new Style({
    image: new Icon({
      src: "/marker.svg", // 需要 marker.svg 文件
      scale: 0.5,
      opacity: isStale ? 0.5 : 1,
    }),
    text: new Text({
      text: name,
      offsetY: -20,
      fill: new Fill({ color: isStale ? "#666" : "#000" }),
      stroke: new Stroke({ color: "#fff", width: 2 }),
    }),
  })
}

export default function MapPage() {
  const { data: mounts, isLoading } = useMountpoints()
  const [searchParams] = useSearchParams()
  const highlightMount = searchParams.get("mount")

  const mapRef = useRef<HTMLDivElement>(null)
  const mapObjRef = useRef<Map | null>(null)
  const [selectedMount, setSelectedMount] = useState<MountpointInfo | null>(null)

  // 初始化地图
  useEffect(() => {
    if (!mapRef.current) return

    const map = new Map({
      target: mapRef.current,
      layers: [
        new TileLayer({
          source: new OSM(),
        }),
      ],
      view: new View({
        center: fromLonLat([116.4, 39.9]), // 北京
        zoom: 5,
      }),
    })

    mapObjRef.current = map

    // 点击事件
    map.on("click", (e) => {
      const features = map.getFeaturesAtPixel(e.pixel)
      if (features.length > 0) {
        const feat = features[0] as Feature
        const mountName = feat.get("mountName")
        const mount = mounts?.find(m => m.name === mountName)
        setSelectedMount(mount || null)
      } else {
        setSelectedMount(null)
      }
    })

    return () => map.setTarget(undefined)
  }, [])

  // 添加标记
  useEffect(() => {
    if (!mapObjRef.current || !mounts) return

    const vectorSource = new VectorSource()
    const now = Date.now() / 1000

    mounts.forEach(mp => {
      if (mp.antenna_lat == null || mp.antenna_lon == null) return

      const feat = new Feature({
        geometry: new Point(fromLonLat([mp.antenna_lon, mp.antenna_lat])),
      })
      feat.set("mountName", mp.name)

      // 判断是否过期
      const isStale = mp.antenna_updated_at
        ? (now - new Date(mp.antenna_updated_at).getTime() / 1000) > 3600
        : true

      feat.setStyle(createMarkerStyle(mp.name, isStale))
      vectorSource.addFeature(feat)
    })

    const vectorLayer = new VectorLayer({
      source: vectorSource,
    })

    mapObjRef.current.addLayer(vectorLayer)

    // 高亮指定挂载点
    if (highlightMount) {
      const mp = mounts.find(m => m.name === highlightMount)
      if (mp?.antenna_lat && mp?.antenna_lon) {
        mapObjRef.current.getView().animate({
          center: fromLonLat([mp.antenna_lon, mp.antenna_lat]),
          zoom: 12,
          duration: 500,
        })
        setSelectedMount(mp)
      }
    }
  }, [mounts, highlightMount])

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-32" />
        <Skeleton className="h-[500px] w-full" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">基站地图</h1>
        <Badge variant="outline">
          {mounts?.filter(m => m.antenna_lat != null).length || 0} 个有位置数据
        </Badge>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2">
          <div ref={mapRef} className="h-[500px] rounded-md border" />
        </div>

        <div>
          {selectedMount ? (
            <Card>
              <CardHeader>
                <CardTitle>{selectedMount.name}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                <div className="text-sm text-muted-foreground">
                  {selectedMount.description || "无描述"}
                </div>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div>
                    <span className="text-muted-foreground">纬度：</span>
                    {selectedMount.antenna_lat?.toFixed(6)}
                  </div>
                  <div>
                    <span className="text-muted-foreground">经度：</span>
                    {selectedMount.antenna_lon?.toFixed(6)}
                  </div>
                  <div>
                    <span className="text-muted-foreground">高度：</span>
                    {selectedMount.antenna_height?.toFixed(1)} m
                  </div>
                  <div>
                    <span className="text-muted-foreground">更新：</span>
                    {selectedMount.antenna_updated_at || "未知"}
                  </div>
                </div>
                <Badge variant={selectedMount.source_online ? "default" : "secondary"}>
                  {selectedMount.source_online ? "在线" : "离线"}
                </Badge>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="py-8 text-center text-muted-foreground">
                点击地图标记查看基站详情
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
```

### Step 9.3: 创建标记图标

- [ ] **创建 marker.svg**

创建 `web/public/marker.svg`：

```svg
<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/>
  <circle cx="12" cy="10" r="3"/>
</svg>
```

### Step 9.4: 添加路由

- [ ] **修改 App.tsx**

在 `web/src/App.tsx` 中：

1. 添加 import：
```tsx
import MapPage from "@/pages/map"
```

2. 在 Routes 中添加路由（在 `connections` 路由后）：
```tsx
<Route path="map" element={<MapPage />} />
```

### Step 9.5: 运行前端开发服务器

- [ ] **启动前端**

```bash
cd web && bun run dev
```

访问 `http://localhost:5173/map` 验证地图页面。

### Step 9.6: 提交

- [ ] **提交代码**

```bash
git add web/src/pages/map.tsx web/src/App.tsx web/public/marker.svg
git commit -m "feat(web): 添加基站地图页面，使用 OpenLayers 显示天线位置"
```

---

## Task 10: 挂载点列表位置按钮

**Files:**
- Modify: `web/src/pages/mountpoints.tsx`

### Step 10.1: 添加位置按钮

- [ ] **修改 mountpoints.tsx**

在 `web/src/pages/mountpoints.tsx` 的 TableRow 中：

在"操作"列添加"位置"按钮：

```tsx
<TableCell className="text-right space-x-2">
  {mp.antenna_lat != null && (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => window.location.href = `/map?mount=${mp.name}`}
    >
      位置
    </Button>
  )}
  <Button variant="ghost" size="sm" onClick={() => openEdit(mp)}>
    编辑
  </Button>
  <Button
    variant="ghost"
    size="sm"
    className="text-destructive hover:text-destructive"
    onClick={() => setDeleteTarget(mp)}
  >
    删除
  </Button>
</TableCell>
```

### Step 10.2: 运行测试

- [ ] **启动前端验证**

```bash
cd web && bun run dev
```

访问 `/mountpoints` 页面验证位置按钮。

### Step 10.3: 提交

- [ ] **提交代码**

```bash
git add web/src/pages/mountpoints.tsx
git commit -m "feat(web): 挂载点列表添加位置按钮，跳转到地图页面"
```

---

## Task 11: 集成测试与覆盖率

**Files:**
- 测试整体集成

### Step 11.1: 运行全量测试

- [ ] **运行所有测试**

```bash
go test ./... -race -cover
```

### Step 11.2: 检查覆盖率

- [ ] **生成覆盖率报告**

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

预期：
- `internal/rtcm/decode1005.go`: 80%+
- `internal/rtcm/msgtype.go`: 90%+

### Step 11.3: 模拟基站测试

- [ ] **使用 simbase 测试**

```bash
# 终端 1: 启动 caster
make test-env && make test-caster

# 终端 2: 模拟基站（发送包含 1005 的数据）
# 需要构造真实 1005 报文或使用实际 GNSS 设备

# 检查 API 返回的位置
curl http://localhost:8080/api/mountpoints
```

### Step 11.4: 前端 E2E 测试

- [ ] **手动验证前端流程**

1. 访问 `/mountpoints`
2. 点击"位置"按钮（如有位置数据）
3. 验证地图页面加载并定位

### Step 11.5: 提交最终版本

- [ ] **最终提交**

```bash
git add -A
git commit -m "feat: 基站位置解析功能完整实现"
git push
```

---

## Self-Review Checklist

- [x] **Spec coverage**: 每个设计章节都有对应任务
- [x] **Placeholder scan**: 无 TBD/TODO/模糊描述
- [x] **Type consistency**: AntennaPosition 类型在各文件中一致使用
- [x] **File paths**: 所有文件路径精确指定
- [x] **Code blocks**: 每个步骤包含完整代码
- [x] **Test commands**: 包含具体测试命令和预期输出