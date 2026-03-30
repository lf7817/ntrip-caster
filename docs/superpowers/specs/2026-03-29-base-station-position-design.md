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

| 步骤 | 任务 | 预估工作量 | 测试要求 |
|------|------|------------|----------|
| 1 | `internal/rtcm/msgtype.go` | ~30 行 | 90%+ 覆盖率 |
| 2 | `internal/rtcm/decode1005.go` | ~100 行 | 80%+ 覆盖率 |
| 3 | `internal/rtcm/decode1005_test.go` | ~80 行 | ECEF 转换 100% |
| 4 | `internal/mountpoint/mountpoint.go` 更新 | ~60 行 | 单元测试防抖逻辑 |
| 5 | `internal/database/database.go` 迁移 | ~20 行 | 验证自动迁移 |
| 6 | `internal/account/service.go` 更新 | ~40 行 | 持久化测试 |
| 7 | `internal/source/source.go` 更新 | ~30 行 | 集成测试 |
| 8 | `internal/api/handlers.go` 更新 | ~20 行 | API 测试 |
| 9 | 前端地图页面 | ~150 行 | E2E 测试 |
| 10 | 前端挂载点列表按钮 | ~30 行 | - |
| 11 | 集成测试 + 覆盖率验证 | - | 总体 80%+ |

**总计：约 550 行代码 + 测试**

**执行顺序：**
1. 后端核心逻辑（步骤 1-7）+ 同步测试
2. API 层（步骤 8）
3. 前端（步骤 9-10）
4. 最终集成测试（步骤 11）

---

## 9. 错误处理策略

### 9.1 解析失败处理

| 场景 | 处理方式 | 日志级别 |
|------|----------|----------|
| RTCM 帧不完整 | 忽略，等待下一帧 | Debug |
| 1005 bit-field 提取失败 | 记录错误，继续广播 | Warn（首次），Debug（后续重复） |
| ECEF 值全为 0 | 忽略（基站未初始化） | Debug |
| ECEF 值超出合理范围 | 记录警告，忽略 | Warn |

**实现原则：**
- 解析失败**不阻塞**广播循环
- 使用 `slog` 结构化日志，包含 mountpoint 名称和错误详情
- 相同错误 5 分钟内只记录一次（防日志泛滥）

### 9.2 坐标转换异常

```go
func ecefToLatLng(x, y, z float64) (lat, lon, h float64, err error) {
    // 检查输入有效性
    if x == 0 && y == 0 && z == 0 {
        return 0, 0, 0, ErrZeroCoordinates
    }

    // 检查是否在地球范围内（半径 ~6378km）
    r := math.Sqrt(x*x + y*y + z*z)
    if r < 6350000 || r > 6420000 {
        return 0, 0, 0, ErrOutOfRange
    }

    // 正常转换...
}
```

### 9.3 数据库持久化失败

- 持久化是**异步操作**，失败不阻塞广播
- 失败时记录 Warn 日志，位置数据仍保留在内存
- 下次成功更新时会再次尝试持久化

---

## 10. 边界情况处理

### 10.1 过期位置

**策略：前端显示"上次更新时间"**

API 返回的 `AntennaPosition` 包含 `UpdatedAt` 时间戳：
- 前端计算 `time.Now() - UpdatedAt`
- > 1 小时：显示黄色警告标记 + "位置数据已过期"
- > 24 小时：显示灰色标记 + "无近期位置数据"

### 10.2 位置跳变检测

**策略：记录但不阻断**

```go
func (m *MountPoint) UpdateAntennaPosition(pos *AntennaPosition) {
    // 计算与上次位置的距离
    if oldPos := m.GetAntennaPosition(); oldPos != nil {
        dist := distance(oldPos, pos)
        if dist > 10000 { // 10km 跳变
            slog.Warn("位置大幅跳变",
                "mount", m.Name,
                "old", oldPos,
                "new", pos,
                "distance_km", dist/1000)
            // 仍然更新，但留下审计日志
        }
    }
    // 继续更新...
}
```

跳变原因可能是：
- 基站物理移动（合法）
- RTCM 数据损坏（需审计）
- 多个数据源冲突（见下节）

### 10.3 多数据源冲突

当前设计：每个挂载点只有一个位置字段，后更新的覆盖前值。

**冲突检测：** 如果同一挂载点短时间内（< 5 秒）收到不同位置：
- 记录 Warn 日志，标注"多源冲突"
- 使用最新值（简单策略）
- 未来可扩展：投票机制、可信源优先级

---

## 11. 热路径性能

### 11.1 当前设计分析

```go
for _, pkt := range packets {
    // 位置解析：约 50ns（ExtractMsgType）+ 500ns（Decode1005）
    if rtcm.ExtractMsgType(pkt) == 1005 {
        pos, err := rtcm.Decode1005(pkt)
        // ...
    }

    // 广播：这是热路径，每帧必走
    s.Mount.Broadcast(pkt)
}
```

**性能估算：**
- 1005 报文频率：通常 1-10 秒一次（不是每帧）
- 非报文帧：仅 `ExtractMsgType` 开销 ~50ns
- 1005 报文帧：额外 ~500ns 解析
- **结论：开销可接受，无需异步化**

### 11.2 保持简单

不引入额外复杂度（异步 channel、worker pool）：
- 1005 报文频率低，解析快
- 异步化会增加竞态条件和调试难度
- YAGNI：当前性能足够，过早优化是浪费

---

## 12. 监控与可观测性

### 12.1 日志策略

| 事件 | 级别 | 内容 |
|------|------|------|
| 首次收到位置 | Info | mount, lat, lon, height |
| 位置更新（5秒防抖后） | Debug | mount, lat, lon |
| 解析失败 | Warn（首次）| mount, error, payload_preview |
| 位置跳变 > 10km | Warn | mount, old, new, distance_km |
| 持久化失败 | Warn | mount, error |

### 12.2 Prometheus 指标（预留）

```go
// internal/metrics/metrics.go（预留）
var (
    positionUpdatesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ntrip_position_updates_total",
            Help: "Total number of antenna position updates",
        },
        []string{"mountpoint"},
    )

    positionParseErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ntrip_position_parse_errors_total",
            Help: "Total number of 1005 parse errors",
        },
        []string{"mountpoint", "error_type"},
    )
)
```

当前版本**仅预留接口**，暂不集成 Prometheus（YAGNI）。

---

## 13. 测试策略

### 13.1 单元测试

**文件：`internal/rtcm/decode1005_test.go`**

```go
func TestExtractMsgType(t *testing.T) {
    tests := []struct {
        name     string
        payload  []byte
        expected uint16
    }{
        {"1005", []byte{0xD3, 0x00, 0x13, 0x10, 0x05, ...}, 1005},
        {"1006", ...},  // 留守扩展
    }
    // ...
}

func TestEcefToLatLng(t *testing.T) {
    // 使用公开的测试向量
    // 例：北京天安门 ECEF → WGS84
    tests := []struct {
        x, y, z    float64
        lat, lon, h float64
    }{
        {-2187800, 4385000, 4071000, 39.9, 116.4, 50}, // 粗略值
        // 可从 GPS 工具网站获取精确测试数据
    }
    // 允许 1 米误差
}
```

### 13.2 集成测试

**文件：`internal/source/source_test.go`**

```go
func TestSourcePositionUpdate(t *testing.T) {
    // 模拟发送包含 1005 报文的 RTCM 流
    // 验证 MountPoint.GetAntennaPosition() 返回正确值
}
```

### 13.3 测试覆盖率目标

- `internal/rtcm/decode1005.go`: **80%+**
- `internal/rtcm/msgtype.go`: **90%+**
- ECEF 转换函数：**100%**（数学函数，必须完全覆盖）

---

## 14. 数据库迁移

### 14.1 迁移策略

**自动迁移（启动时）**

```go
// internal/database/database.go
func (db *Database) migrate() error {
    // 现有迁移...

    // 新增：天线位置字段
    _, err := db.Exec(`
        ALTER TABLE mountpoints ADD COLUMN antenna_lat REAL DEFAULT NULL;
        ALTER TABLE mountpoints ADD COLUMN antenna_lon REAL DEFAULT NULL;
        ALTER TABLE mountpoints ADD COLUMN antenna_height REAL DEFAULT NULL;
        ALTER TABLE mountpoints ADD COLUMN antenna_updated_at INTEGER DEFAULT NULL;
    `)
    if err != nil && !isDuplicateColumnError(err) {
        return err
    }
    return nil
}
```

### 14.2 兼容性

- 新列 `DEFAULT NULL`：现有挂载点无位置数据
- `isDuplicateColumnError`：已迁移的数据库启动不报错
- 无需单独迁移脚本，启动时自动处理

---

## 15. 扩展预留

当前设计预留了向方案 C（可扩展解析框架）演进的能力：

1. `MsgTypeParser` 接口定义
2. `RegisterParser()` 注册机制
3. `registeredParsers` map 存储解析器

未来添加新消息类型（如 1033）只需：
- 实现 `MsgTypeParser` 接口
- 调用 `RegisterParser()` 注册
- 在读循环中调用解析逻辑