# 限流方案设计

## 概述

NTRIP Caster 实现两级限流机制：全局级别和挂载点级别，保护服务器资源，防止单个 IP 或单个挂载点占用过多连接。

## 限流层级

```
┌─────────────────────────────────────────────────────────────┐
│                        TCP 连接                              │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  第一层：IP 限流 (max_conn_per_ip)                           │
│  - 检查时机：TCP 建立时                                       │
│  - 作用范围：所有连接（Source + Rover）                       │
│  - 超限处理：直接关闭 TCP 连接                                │
└─────────────────────────────────────────────────────────────┘
                              │ 通过
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    解析 NTRIP 请求                           │
│                  判断连接类型 (SOURCE/GET)                    │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────────┐
│       SOURCE 连接        │     │         ROVER 连接          │
│   无额外限流，直接处理    │     └─────────────────────────────┘
└─────────────────────────┘                   │
                                              ▼
                            ┌─────────────────────────────────┐
                            │  第二层：全局客户端限流           │
                            │  (max_clients)                  │
                            │  - 检查时机：Rover 连接时         │
                            │  - 作用范围：全局 Rover 总数       │
                            │  - 超限处理：返回 503             │
                            └─────────────────────────────────┘
                                              │ 通过
                                              ▼
                            ┌─────────────────────────────────┐
                            │  第三层：挂载点客户端限流         │
                            │  (mountpoint.max_clients)       │
                            │  - 检查时机：Rover 加入挂载点时   │
                            │  - 作用范围：单挂载点 Rover 数    │
                            │  - 超限处理：返回 503             │
                            └─────────────────────────────────┘
                                              │ 通过
                                              ▼
                            ┌─────────────────────────────────┐
                            │          正常处理连接            │
                            └─────────────────────────────────┘
```

## 配置项说明

### 全局配置（config.yaml）

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `limits.max_clients` | int | 5000 | 全局 Rover 连接总数上限 |
| `limits.max_conn_per_ip` | int | 10 | 单 IP 并发连接数上限（含 Source 和 Rover） |

```yaml
limits:
  max_clients: 5000      # 全局 Rover 总数
  max_conn_per_ip: 10    # 单 IP 连接数
```

### 挂载点配置（数据库）

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_clients` | INTEGER | 0 | 单挂载点 Rover 连接数上限，0 表示无限制 |

```sql
-- mountpoints 表新增字段
ALTER TABLE mountpoints ADD COLUMN max_clients INTEGER DEFAULT 0;
```

## 检查逻辑

### 1. IP 限流

**位置**：`internal/caster/server.go` - TCP Accept 时

**逻辑**：
```go
func (s *Server) handleAccept(conn net.Conn) {
    ip := extractIP(conn.RemoteAddr())
    if !s.limiter.Allow(ip) {
        conn.Close()  // 直接关闭，不发送任何响应
        return
    }
    defer s.limiter.Release(ip)
    // 继续处理连接...
}
```

**特点**：
- 最早拦截，保护服务器资源
- 不区分 Source/Rover，统一计数
- 超限直接关闭 TCP，无 HTTP 响应

### 2. 全局客户端限流

**位置**：`internal/caster/connection.go` - handleRover 时

**逻辑**：
```go
func (h *connHandler) handleRover(req *NTRIPRequest, conn net.Conn) {
    // 检查全局 Rover 数量
    if h.globalLimiter.ClientCount() >= h.cfg.Limits.MaxClients {
        sendError(conn, 503, "Service Unavailable: server at capacity")
        return
    }

    h.globalLimiter.AddClient()
    defer h.globalLimiter.ReleaseClient()
    // 继续处理...
}
```

**特点**：
- 只限制 Rover，Source 不受限
- 超限返回 HTTP 503 错误
- 客户端可以收到明确错误信息

### 3. 挂载点客户端限流

**位置**：`internal/mountpoint/mountpoint.go` - AddClient 时

**逻辑**：
```go
func (m *MountPoint) AddClient(c *client.Client) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // 检查挂载点连接数限制
    if m.maxClients > 0 && len(m.clientsByID) >= m.maxClients {
        return ErrClientLimitReached
    }

    m.clientsByID[c.ID] = c
    m.rebuildSnapshotLocked()
    m.Stats.ClientCount.Store(int64(len(m.clientsByID)))
    return nil
}
```

**特点**：
- 每个挂载点可独立配置
- 0 表示无限制
- 超限返回错误，调用方返回 503

## 错误响应

| 场景 | HTTP 状态码 | 响应内容 |
|------|-------------|----------|
| IP 连接数超限 | 无响应 | 直接关闭 TCP |
| 全局 Rover 数超限 | 503 | `Service Unavailable: server at capacity` |
| 挂载点 Rover 数超限 | 503 | `Service Unavailable: mountpoint at capacity` |

## 数据结构变更

### 1. 全局限流器

**新增**：`internal/limiter/global_limiter.go`

```go
package limiter

import "sync/atomic"

// GlobalLimiter tracks global client counts.
type GlobalLimiter struct {
    clientCount atomic.Int64
    maxClients  int
}

func NewGlobalLimiter(maxClients int) *GlobalLimiter {
    return &GlobalLimiter{maxClients: maxClients}
}

func (l *GlobalLimiter) ClientCount() int64 {
    return l.clientCount.Load()
}

func (l *GlobalLimiter) AddClient() {
    l.clientCount.Add(1)
}

func (l *GlobalLimiter) ReleaseClient() {
    l.clientCount.Add(-1)
}

func (l *GlobalLimiter) AtCapacity() bool {
    return l.clientCount.Load() >= int64(l.maxClients)
}
```

### 2. 挂载点结构

**修改**：`internal/mountpoint/mountpoint.go`

```go
type MountPoint struct {
    Name         string
    Description  string
    Format       string
    enabled      atomic.Bool
    WriteQueue   int
    WriteTimeout time.Duration
    MaxClients   int  // 新增：挂载点连接数上限，0 表示无限制

    mu          sync.Mutex
    source      *SourceInfo
    clientsByID map[string]*client.Client
    snapshot    atomic.Value

    Stats metrics.MountStats
}
```

### 3. 数据库表结构

**修改**：`internal/database/database.go`

```sql
CREATE TABLE IF NOT EXISTS mountpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    format TEXT DEFAULT 'RTCM3',
    enabled INTEGER DEFAULT 1,
    write_queue INTEGER DEFAULT 64,
    write_timeout_ms INTEGER DEFAULT 3000,
    max_clients INTEGER DEFAULT 0,  -- 新增
    secret TEXT
);
```

## API 变更

### 创建/更新挂载点

**请求体新增字段**：

```json
{
  "name": "MOUNT01",
  "description": "测试挂载点",
  "max_clients": 100
}
```

### 挂载点列表响应

**响应体新增字段**：

```json
{
  "id": 1,
  "name": "MOUNT01",
  "description": "测试挂载点",
  "max_clients": 100,
  "source_online": true,
  "client_count": 50
}
```

## 前端变更

### 挂载点管理页面

**表单新增**：
- 最大客户端数输入框
- 提示文字："0 表示无限制"

**列表显示**：
- 当前连接数 / 最大连接数：`50 / 100` 或 `50 / ∞`（max_clients = 0）

## 实现清单

- [x] `internal/limiter/global_limiter.go` - 新增全局限流器
- [x] `internal/database/database.go` - 表结构添加 `max_clients` 字段
- [x] `internal/mountpoint/mountpoint.go` - 添加 `MaxClients` 字段和检查逻辑
- [x] `internal/caster/server.go` - 集成全局限流器
- [x] `internal/caster/connection.go` - Rover 连接时检查全局和挂载点限制
- [x] `internal/api/handlers.go` - API 支持设置 `max_clients`
- [x] `internal/account/service.go` - 数据库 CRUD 支持新字段
- [x] `web/src/api/types.ts` - 类型定义添加 `max_clients`
- [x] `web/src/pages/mountpoints.tsx` - 表单和列表显示

## 测试要点

1. **IP 限流测试**
   - 同一 IP 建立超过 max_conn_per_ip 个连接
   - 验证超限连接被直接关闭

2. **全局客户端限流测试**
   - 建立接近 max_clients 个 Rover 连接
   - 验证超限连接收到 503 响应
   - Source 连接不受影响

3. **挂载点客户端限流测试**
   - 设置挂载点 max_clients = 10
   - 建立 10 个连接后验证第 11 个被拒绝
   - 其他挂载点不受影响

4. **配置值为 0 测试**
   - 挂载点 max_clients = 0 时无限制