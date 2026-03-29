# 基站位置解析设计文档

**日期：** 2026-03-29
**状态：** 已确认，待实现

---

## 1. 功能概述

解析 RTCM 1005 报文，提取基站天线位置（ECEF 坐标），转换为经纬度，存储并显示在 Web 界面地图上。

---

## 2. 坐标系说明

### RTCM 1005 原始数据

**坐标系：ECEF（Earth-Centered, Earth-Fixed，地心地固坐标系）**

- 原点：地球质心
- X轴：指向赤道面与本初子午线（0°经线）交点
- Y轴：指向赤道面与东经90°交点
- Z轴：指向北极方向
- 单位：米（38-bit signed integer，分辨率约 0.5mm）

这是 GNSS 系统（GPS、北斗、GLONASS、Galileo）的标准坐标系。

### 转换过程

```
RTCM 1005
    │
    ▼
ECEF (X, Y, Z) ← 地心地固坐标，单位米
    │
    ▼ 使用 WGS84 椭球参数转换
    │
WGS84 (Lat, Lon, Height) ← 经纬度 + 大地高
    │
    ▼ 前端地图使用
    │
WebMercator (EPSG:3857) ← OpenLayers 默认投影
```

**WGS84 参数：**
- 长半轴 a = 6378137.0 米
- 扁率 f = 1/298.257223563
- 短半轴 b = a × (1 - f)

### 为什么选择 WGS84

1. **GNSS 标准兼容** — GPS、北斗、Galileo 都用 WGS84
2. **地图兼容** — OpenStreetMap 底图使用 WGS84/WebMercator
3. **转换简单** — ECEF → WGS84 转换公式成熟，无额外投影转换开销

---

## 3. 模块设计

### 3.1 RTCM 消息类型解析

**文件：`internal/rtcm/msgtype.go`**

```go
package rtcm

// ExtractMsgType 从 RTCM3 帧中提取消息类型（DF002）
// RTCM3 帧结构: 0xD3 | len(10bit) | payload | CRC
// 消息类型在 payload 的前 12 bit（byte 3-4）
func ExtractMsgType(pkt *RTCMPacket) uint16

// MsgTypeParser 定义消息解析器接口（预留扩展）
type MsgTypeParser interface {
    MsgType() uint16
    Parse(payload []byte) (any, error)
}

// 已注册的解析器（预留扩展）
var registeredParsers = make(map[uint16]MsgTypeParser)

func RegisterParser(p MsgTypeParser)
```

### 3.2 RTCM 1005 解析

**文件：`internal/rtcm/decode1005.go`**

```go
package rtcm

// AntennaPosition 表示基站天线位置（经纬度格式）
type AntennaPosition struct {
    Latitude  float64 // 度
    Longitude float64 // 度
    Height    float64 // 米（天线高度）
    UpdatedAt int64   // 更新时间戳（Unix seconds）
}

// Decode1005 解析 RTCM 1005 报文
func Decode1005(pkt *RTCMPacket) (*AntennaPosition, error)

// ecefToLatLng 将 ECEF 坐标转换为经纬度（WGS84）
func ecefToLatLng(x, y, z float64) (lat, lon, h float64)
```

**1005 bit field 解析细节：**
- 需要精确处理 38-bit signed integer（ECEF 坐标）
- 使用 bit 操作提取字段，而非标准 binary 包

---

## 4. 数据模型变更

### 4.1 MountPoint 结构

**文件：`internal/mountpoint/mountpoint.go`**

新增字段：
```go
type MountPoint struct {
    // ... 现有字段 ...

    // 新增：天线位置
    antennaPos *AntennaPositionInfo // nil 表示无位置数据
}

type AntennaPositionInfo struct {
    Position    AntennaPosition
    LastUpdate  atomic.Int64 // 上次更新时间戳（防抖用）
    mu          sync.Mutex
}
```

方法：
```go
// UpdateAntennaPosition 更新天线位置（带 5 秒防抖）
func (m *MountPoint) UpdateAntennaPosition(pos *AntennaPosition)

// GetAntennaPosition 返回当前天线位置（线程安全）
func (m *MountPoint) GetAntennaPosition() *AntennaPosition
```

### 4.2 数据库变更

**文件：`internal/database/database.go`**

新增字段：
```sql
ALTER TABLE mountpoints ADD COLUMN antenna_lat REAL;
ALTER TABLE mountpoints ADD COLUMN antenna_lon REAL;
ALTER TABLE mountpoints ADD COLUMN antenna_height REAL;
ALTER TABLE mountpoints ADD COLUMN antenna_updated_at INTEGER;
```

**文件：`internal/account/service.go`**

新增方法：
```go
// UpdateMountPointAntennaPosition 持久化天线位置
func (s *Service) UpdateMountPointAntennaPosition(name string, pos *AntennaPosition) error

// GetMountPointAntennaPosition 读取历史位置
func (s *Service) GetMountPointAntennaPosition(name string) (*AntennaPosition, error)
```

---

## 5. 处理流程

### 5.1 Source 读循环

**文件：`internal/source/source.go`**

```go
func (s *Source) ReadLoop() {
    framer := &rtcm.RTCMFramer{}

    for {
        packets := framer.Push(buf[:n])
        for _, pkt := range packets {
            // 检查是否为 1005 报文
            if rtcm.ExtractMsgType(pkt) == 1005 {
                pos, err := rtcm.Decode1005(pkt)
                if err == nil && pos != nil {
                    s.Mount.UpdateAntennaPosition(pos)
                    // 异步持久化，不阻塞广播
                    go s.persistPosition(pos)
                }
            }

            // 广播逻辑不变
            s.Mount.Broadcast(pkt)
        }
    }
}
```

### 5.2 防抖策略

- 收到 1005 报文后更新位置
- 5 秒内只更新一次，避免高频报文频繁更新
- 位置同时保存到内存（实时显示）和数据库（持久化）

---

## 6. API 变更

**文件：`internal/api/handlers.go`**

`ListMountpoints` 返回新增字段：
```go
type mpInfo struct {
    // ... 现有字段 ...

    AntennaLat     float64 `json:"antenna_lat,omitempty"`
    AntennaLon     float64 `json:"antenna_lon,omitempty"`
    AntennaHeight  float64 `json:"antenna_height,omitempty"`
    AntennaUpdated string  `json:"antenna_updated_at,omitempty"`
}
```

---

## 7. 前端设计

### 7.1 地图页面

**文件：`web/src/pages/map.tsx`**

- 使用 OpenLayers + OSM 底图
- 显示所有有位置数据的基站标记
- 支持点击标记查看详细信息

**依赖：**
```bash
bun add ol
```

### 7.2 挂载点列表

**文件：`web/src/pages/mountpoints.tsx`**

新增"查看位置"按钮：
- 仅当有位置数据时可点击
- 点击跳转到 `/map?mount={name}` 并定位

### 7.3 路由

**文件：`web/src/App.tsx`**

新增路由：
```tsx
<Route path="/map" element={<MapPage />} />
```

---

## 8. 实现计划

| 步骤 | 任务 | 预估工作量 |
|------|------|------------|
| 1 | `internal/rtcm/msgtype.go` | ~30 行 |
| 2 | `internal/rtcm/decode1005.go` | ~100 行 |
| 3 | `internal/mountpoint/mountpoint.go` 更新 | ~60 行 |
| 4 | `internal/database/database.go` 更新 | ~10 行 |
| 5 | `internal/account/service.go` 更新 | ~40 行 |
| 6 | `internal/source/source.go` 更新 | ~30 行 |
| 7 | `internal/api/handlers.go` 更新 | ~20 行 |
| 8 | 前端地图页面 | ~150 行 |
| 9 | 前端挂载点列表按钮 | ~30 行 |
| 10 | 集成测试 | - |

**总计：约 400 行代码**

---

## 9. 扩展预留

当前设计预留了向方案 C（可扩展解析框架）演进的能力：

1. `MsgTypeParser` 接口定义
2. `RegisterParser()` 注册机制
3. `registeredParsers` map 存储解析器

未来添加新消息类型（如 1033）只需：
- 实现 `MsgTypeParser` 接口
- 调用 `RegisterParser()` 注册
- 在读循环中调用解析逻辑