# Go NTRIP Caster 架构设计

> 适用于构建一个支持 **多基站、多 Rover、账号管理、Mountpoint 管理、Web
> 管理界面** 的完整 NTRIP Caster。\
> 协议：NTRIP (Networked Transport of RTCM via Internet Protocol)

生成时间：2026-03-13T13:37:23.736202 UTC

------------------------------------------------------------------------

# 1. 系统目标

设计一个生产级 NTRIP Caster，支持：

-   多 Base Station (NTRIP Source)
-   多 Rover Client
-   Mountpoint 管理
-   用户账号与权限管理
-   Web 管理 API
-   高并发稳定 RTCM 转发
-   实时统计与监控

目标性能：

-   1 vCPU 支持 2000--5000 Rover（经验目标，需压测验证）
-   广播链路延迟目标 \< 50ms（不含公网抖动）
-   RTCM 广播无阻塞

------------------------------------------------------------------------

# 2. 总体系统架构

                    ┌──────────────────────┐
                    │       Web UI         │
                    │  Admin Dashboard     │
                    └─────────┬────────────┘
                              │ HTTP
                              ▼
                    ┌──────────────────────┐
                    │   Admin API Module   │
                    │ Users / Mountpoints  │
                    └─────────┬────────────┘
                              │ in-process
                              ▼
                    ┌──────────────────────┐
                    │     Caster Core      │
                    │  NTRIP TCP Server    │
                    └─────────┬────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
        Base Stations                    Rover Clients
       (NTRIP Source)                  (NTRIP Client)

核心思想：

-   **默认形态是单进程、模块化单体（modular monolith）**
-   **NTRIP 数据面与管理面逻辑隔离，但默认不拆成两个独立服务**
-   **后续如果需要水平扩展，再演进为 Control Plane / Data Plane 分离架构**

说明：

-   本文默认实现为一个 Go 二进制：一个监听 NTRIP 端口，一个监听 Admin HTTP 端口。
-   这样能减少配置同步、注册发现、跨进程故障恢复等复杂度，更适合首个生产版本。
-   如果未来需要多节点集群，再把 Mountpoint 配置、用户管理、统计聚合拆到独立控制面。

------------------------------------------------------------------------

# 3. 项目目录结构（推荐）

    ntrip-caster
    │
    ├── cmd
    │   └── caster
    │        main.go
    │
    ├── internal
    │
    │   ├── caster
    │   │     server.go
    │   │     connection.go
    │   │     protocol.go
    │   │
    │   ├── mountpoint
    │   │     manager.go
    │   │     mountpoint.go
    │   │
    │   ├── source
    │   │     source.go
    │   │
    │   ├── client
    │   │     client.go
    │   │
    │   ├── auth
    │   │     auth.go
    │   │
    │   ├── account
    │   │     user.go
    │   │     service.go
    │   │
    │   ├── sourcetable
    │   │     generator.go
    │   │
    │   ├── api
    │   │     http_server.go
    │   │     handlers.go
    │   │
    │   └── metrics
    │         stats.go
    │
    └── pkg
          logger

------------------------------------------------------------------------

# 4. 核心数据模型

## User

    type User struct {
        ID           int64
        Username     string
        PasswordHash string   // 仅存 bcrypt/argon2 哈希，不存明文
        Role         string
        Enabled      bool
    }

角色：

-   admin
-   base
-   rover

------------------------------------------------------------------------

## MountPoint

    type MountPoint struct {
        Name        string
        Description string
        Enabled     bool      // 与数据库 mountpoints.enabled 一致，禁用则不列入 Sourcetable、不可挂载
        WriteQueue  int       // 可选，覆盖全局 client_write_queue
        WriteTimeout time.Duration // 可选，覆盖全局 client_write_timeout

        Source  *Source
        Clients map[string]*Client

        mu sync.RWMutex

        Stats MountStats
    }

高并发优化版（推荐用于生产）：

    type MountPoint struct {
        Name        string
        Description string
        Enabled     bool

        Source *Source

        mu          sync.Mutex
        clientsByID map[string]*Client
        snapshot    atomic.Value // []*Client，只读快照，供广播路径无锁读取

        Stats MountStats
    }

说明：

-   默认版更容易实现，适合第一版。
-   如果广播频率高、客户端很多，推荐维护 `clientsByID + snapshot` 双结构。
-   **读路径无锁，写路径串行**，比“每次广播在锁内遍历 map 再构建快照”更适合 fanout 热路径。

------------------------------------------------------------------------

## Source

    type Source struct {
        ID string
        Conn net.Conn
        Mount *MountPoint
        StartTime time.Time
    }

------------------------------------------------------------------------

## RTCMPacket

    type RTCMPacket struct {
        Data []byte // 只读；创建后不可修改
    }

说明：

-   `RTCMPacket` 只是对共享 `[]byte` 的轻量封装，不是额外做一层 payload copy。
-   如果项目当前阶段只追求最小实现，也可以直接用共享 `[]byte`；两者在 fanout 拷贝策略上等价。
-   保留该结构体的目的主要是表达“只读广播包”语义，并为后续扩展时间戳、序号、池化元数据预留位置。

------------------------------------------------------------------------

## Client

    type Client struct {
        ID string
        Conn net.Conn
        Mount *MountPoint
        WriteChan chan *RTCMPacket
        Done chan struct{}
        CloseOnce sync.Once
        ConnectedAt time.Time
    }

------------------------------------------------------------------------

# 5. 连接生命周期

## Rover Client

连接流程：

    TCP Accept
       ↓
    Parse HTTP GET
       ↓
    Basic Auth
       ↓
    Mountpoint 查找
       ↓
    注册 Client
       ↓
    启动 writeLoop

失败响应建议：

-   未认证：返回 `401 Unauthorized`
-   已认证但无权限：返回 `403 Forbidden`
-   Mountpoint 不存在或已禁用：返回 `404 Not Found` 或兼容设备所需的最小错误响应
-   文档与代码应统一错误响应策略，优先保证常见 NTRIP 设备兼容性

------------------------------------------------------------------------

## Base Source

连接流程：

    TCP Accept
       ↓
    识别 SOURCE(Rev1) 或 POST(Rev2)
       ↓
    认证 Source 身份
       ↓
    注册 Mountpoint Source
       ↓
    接收 RTCM
       ↓
    广播 RTCM

协议要求：

-   **Rover** 至少支持标准 HTTP GET 挂载 Mountpoint。
-   **Source** 建议同时支持：
    - NTRIP Rev1: `SOURCE password /mountpoint`
    - NTRIP Rev2: `POST /mountpoint HTTP/1.1`
-   文档、代码、测试都应明确支持范围；不要只写 `SOURCE`，否则容易误导为“仅支持 Rev1”或“把 Rev1 当标准全貌”。

认证建议：

-   不建议使用“挂载点明文密码”这种难审计的模型。
-   推荐统一为 **Source 用户账号** 登录，并在数据库中绑定可发布的 Mountpoint。
-   若必须兼容传统设备，可额外提供 `mountpoint source secret` 兼容模式，但数据库中只保存哈希值。

Source 断线时：

-   从 MountPoint 上摘掉当前 Source（Mount.Source = nil），不持写锁做耗时操作。
-   Rover 侧表现为无新数据；可允许同一 Mountpoint 在 Base 重连后自动再次挂上。
-   清理在 Conn 读循环退出或 Conn 关闭的 goroutine 里执行，避免在广播路径持锁移除。
-   必须保证与该 Source 关联的 goroutine 能收敛退出，避免 readLoop、鉴权超时协程、统计协程泄漏。
-   推荐在连接对象上统一维护 `Done`/`Context`，关闭连接时广播退出信号。

------------------------------------------------------------------------

# 6. RTCM Fanout 架构（核心设计）

错误设计：

    for client {
      conn.Write(data)
    }

正确设计：

    Source
      │
      ▼
    Mountpoint Broadcast
      │
      ├── client channel
      ├── client channel
      └── client channel

推荐设计：

-   **不要对每个 client 单独拷贝 RTCM 数据**
-   **只在 Source 入站时拷贝一次**，把该包视为只读对象，然后广播同一个 `*RTCMPacket` 给多个客户端
-   **不要在广播路径关闭 `WriteChan`**，否则会出现“已关闭 channel 再发送”的 panic

这与“共享 `[]byte` 广播”在性能上的核心收益相同：

-   Base -> Mountpoint：1 次 payload copy
-   Mountpoint -> 多 Client：0 次 payload copy

Source 读循环（如果底层读缓冲会复用，就在这里做一次克隆）：

    func (s *Source) readLoop() {
        buf := make([]byte, 4096)
        for {
            n, err := s.Conn.Read(buf)
            if err != nil {
                return
            }

            pkt := &RTCMPacket{
                Data: bytes.Clone(buf[:n]), // 每个入站包只拷贝一次，而不是按 client 拷贝
            }
            s.Mount.Broadcast(pkt)
        }
    }

广播路径（默认版，易实现）：

    func (m *MountPoint) Broadcast(pkt *RTCMPacket) {
        m.mu.RLock()
        clients := make([]*Client, 0, len(m.Clients))
        for _, c := range m.Clients {
            clients = append(clients, c)
        }
        m.mu.RUnlock()

        for _, c := range clients {
            select {
            case c.WriteChan <- pkt:
            case <-c.Done:
            default:
                go c.KickSlowConsumer()
            }
        }
    }

广播路径（优化版，推荐）：

    func (m *MountPoint) Broadcast(pkt *RTCMPacket) {
        clients, _ := m.snapshot.Load().([]*Client)
        for _, c := range clients {
            select {
            case c.WriteChan <- pkt:
            case <-c.Done:
            default:
                go c.KickSlowConsumer()
            }
        }
    }

快照维护（写侧串行，避免丢更新）：

    func (m *MountPoint) addClient(c *Client) {
        m.mu.Lock()
        defer m.mu.Unlock()

        m.clientsByID[c.ID] = c
        m.rebuildSnapshotLocked()
    }

    func (m *MountPoint) removeClient(id string) {
        m.mu.Lock()
        defer m.mu.Unlock()

        delete(m.clientsByID, id)
        m.rebuildSnapshotLocked()
    }

    func (m *MountPoint) rebuildSnapshotLocked() {
        next := make([]*Client, 0, len(m.clientsByID))
        for _, c := range m.clientsByID {
            next = append(next, c)
        }
        m.snapshot.Store(next)
    }

客户端关闭与写线程：

    func (c *Client) KickSlowConsumer() {
        c.CloseOnce.Do(func() {
            close(c.Done)
            _ = c.Conn.Close()
        })
    }

    func (c *Client) writeLoop() {
        defer c.removeFromMount()
        for {
            select {
            case <-c.Done:
                return
            case pkt := <-c.WriteChan:
                _, err := c.Conn.Write(pkt.Data)
                if err != nil {
                    c.KickSlowConsumer()
                    return
                }
            }
        }
    }

优势：

-   慢客户端不会拖慢系统
-   无阻塞 fanout
-   每个 RTCM 包只分配/拷贝一次，显著降低 GC 压力
-   共享只读 `RTCMPacket`，避免 per-client payload copy
-   不会因为“关闭 channel 但尚未从 map 移除”而触发 panic

工程注意点：

-   **不要直接用** `old := snapshot; new := append(old, c)` 这种方式更新原子快照；`append` 可能复用旧底层数组，破坏快照不可变性。
-   `atomic.Value` 只负责读路径无锁，不负责写路径并发安全；增删 client 仍要用互斥锁串行化，否则会丢更新。
-   `Broadcast()` 不要在持有 `MountPoint` 读锁时执行任何可能阻塞的关闭或移除逻辑。
-   `removeFromMount()` 才是修改 `m.Clients` 的唯一入口，并由写锁保护。
-   每个客户端都应设置写超时，例如 `SetWriteDeadline`，避免内核缓冲长期阻塞。
-   `WriteChan` 缓冲区大小要配置化，例如 32/64/128 条消息，根据 RTCM 包大小和慢客户端容忍度调优。
-   全局默认值之外，可允许 `MountPoint` 单独覆盖 `WriteChan` 容量和写超时，便于针对高频/低频挂载点调优。
-   共享广播的前提是 `RTCMPacket.Data` 创建后绝不再修改。
-   如果团队更偏好极简实现，可先用 `chan []byte` + 只读约定；当需要更多元数据时再升级为 `RTCMPacket`。
-   如果后续压测发现“每包一次分配”仍是热点，再演进为 `sync.Pool + 引用计数`；第一版不要为了省分配把生命周期管理写复杂。
-   对慢客户端建议增加计数和监控，而不是只在日志里打印；否则高峰期会放大日志噪声与调试成本。

------------------------------------------------------------------------

# 7. Mountpoint Manager

    type Manager struct {
        mounts map[string]*MountPoint
        mu sync.RWMutex
    }

主要方法：

-   Get()
-   Create()
-   Delete()
-   List()
-   RemoveClient(mountName, clientID)  // 由 Client.writeLoop 退出或连接断开时调用，持写锁从 Mount 移除

说明：

-   如果采用优化版 `snapshot` 结构，`AddClient/RemoveClient` 除了维护 `clientsByID`，还需要同步重建只读快照。
-   删除 Mountpoint 时，先禁用新连接，再摘掉 Source、断开现有 Clients，最后从 Manager 移除，避免广播路径访问半销毁对象。

------------------------------------------------------------------------

# 8. Sourcetable 自动生成

当客户端请求：

    GET /

返回：

    SOURCETABLE 200 OK
    STR;MOUNT1;RTCM3;...
    STR;MOUNT2;RTCM3;...
    ENDSOURCETABLE

数据来源与规则：

-   Mountpoint Manager
-   仅包含 Enabled == true 的 Mountpoint

优化建议：

-   第一版可按请求实时生成。
-   如果后续 `GET /` 请求频繁，可增加短 TTL 缓存（例如 1--5 秒）或在 Mountpoint 变更时主动失效。
-   不建议首版就为 Sourcetable 引入 gzip 或复杂缓存层，除非压测证明它是热点。

------------------------------------------------------------------------

# 9. Web API

示例接口：

    POST /api/login
    GET  /api/users
    GET  /api/mountpoints
    GET  /api/sources
    GET  /api/clients

最少还需要明确：

-   Admin API 使用 Session 或 JWT 二选一，不要只写 `/api/login`
-   所有管理接口必须鉴权，并按角色授权
-   登录失败限流与审计日志
-   修改用户密码、禁用账号、踢下线 Source/Client 的接口
-   Mountpoint 启停、Source 绑定、只读查看权限

------------------------------------------------------------------------

# 10. 数据库设计

默认推荐：SQLite

说明：

-   首版项目优先使用 SQLite，部署简单、备份方便，足以承载用户、Mountpoint、绑定关系等控制面数据。
-   当后续出现多实例控制面、审计日志显著增长或管理面并发明显升高时，再迁移到 PostgreSQL。

## users

    id
    username
    password_hash   // bcrypt/argon2，永不存明文
    role
    enabled

## mountpoints

    id
    name
    description
    enabled
    format              // 如 RTCM3
    source_auth_mode    // user_binding / secret
    source_secret_hash  // 可空，仅兼容传统设备时使用
    write_queue         // 可空，覆盖全局默认值
    write_timeout_ms    // 可空，覆盖全局默认值

## user_mountpoint_bindings

    id
    user_id
    mountpoint_id
    permission          // publish / subscribe / admin

设计说明：

-   `base` 用户是否能推流到某个 Mountpoint，必须由绑定关系决定，而不是靠用户名约定。
-   `rover` 用户是否能订阅某个 Mountpoint，也建议支持绑定或 ACL。
-   如果项目早期不做细粒度 ACL，至少也要在文档中明确“当前版本为全局订阅权限”。

------------------------------------------------------------------------

# 11. Metrics

统计数据：

Mountpoint：

-   client_count
-   source_online
-   bytes_in
-   bytes_out
-   slow_clients
-   kick_count
-   write_queue_depth

全局：

-   total_clients
-   total_sources
-   traffic

可选高级指标：

-   client_write_latency
-   broadcast_drop_count
-   source_reconnect_count

建议接入：

-   Prometheus
-   Grafana

------------------------------------------------------------------------

# 12. 配置文件

    server:
      listen: ":2101"       # NTRIP 唯一端口
      admin_listen: ":8080" # Web 管理 API/UI，与 NTRIP 分离

    auth:
      enabled: true
      admin_mode: session
      ntrip_source_auth: user_binding   # user_binding / secret
      ntrip_rover_auth: basic           # none / basic

    database:
      type: sqlite
      path: caster.db

    limits:
      max_clients: 5000     # 全局 Rover 上限（非 per-mountpoint）
      client_write_queue: 64
      client_write_timeout: 3s

    mountpoint_defaults:
      write_queue: 64
      write_timeout: 3s

------------------------------------------------------------------------

# 13. 性能目标

服务器：

-   1 vCPU
-   4GB RAM

能力：

-   10 Base
-   2000--5000 Rover（依赖 RTCM 平均包速率、客户端网络质量、写队列大小、写超时策略）

RTCM 流量：

-   单机/单进程建议 \< 10MB/s，与 max_clients 配合评估

说明：

-   `1 vCPU 3000--5000 Rover` 只能视为经验目标，不能当容量承诺。
-   上线前必须通过压测验证：连接数、广播延迟、丢弃率、内存占用、GC 停顿。
-   建议把慢客户端踢除率纳入监控，否则吞吐看起来正常但用户体验会恶化。

压测建议：

-   使用真实 RTCM 流量或接近真实大小/频率的模拟流量。
-   同时覆盖多 Mountpoint、多 Source、多类 Rover 网络质量。
-   刻意注入慢客户端，观察 `kick_count`、`write_queue_depth`、内存占用与 GC 变化。
-   记录广播延迟分布，而不是只看平均值。

------------------------------------------------------------------------

# 14. 部署架构

    Internet
       │
       ▼
    LoadBalancer
       │
       ▼
    NTRIP Caster
       │
       ▼
    Database

端口：

-   2101  NTRIP（Base/Rover/GET  sourcetable）
-   8080  Web 管理 API/UI（可选，与 NTRIP 同机时建议分离）

------------------------------------------------------------------------

# 15. 实现规模

  模块            代码量
  --------------- ---------
  NTRIP协议解析   \~300行
  Caster核心      \~600行
  账号系统        \~400行
  API             \~400行
  统计与工具      \~200行

总计：

**约 2000 行 Go 代码**

------------------------------------------------------------------------

# 16. 未来可扩展

-   TLS NTRIP
-   集群 Caster
-   Rover 地图
-   Base 自动注册
-   流量统计计费
