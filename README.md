# NTRIP Caster

一个生产级的 NTRIP（Networked Transport of RTCM via Internet Protocol）Caster，使用 Go 编写。支持将 RTCM 差分数据从基站高并发、无阻塞地转发至多个 Rover 客户端。

## 功能特性

- **NTRIP Rev1 & Rev2** -- 基站支持 `SOURCE`（Rev1）和 `POST`（Rev2）；Rover 客户端支持 `GET`
- **高性能广播** -- 基于 atomic snapshot 的无锁广播，RTCM 包零拷贝分发
- **RTCM3 帧解析** -- TCP 字节流重组为完整 RTCM3 帧后再广播，保证数据完整性
- **慢客户端踢除** -- CAS 防抖机制，可配置写队列深度和超时时间
- **用户与挂载点管理** -- 基于角色的账号体系（admin / base / rover），bcrypt 密码哈希
- **挂载点级 ACL** -- 用户-挂载点绑定关系，支持 publish / subscribe 权限控制
- **SQLite 存储** -- 零配置嵌入式数据库（纯 Go 实现，无需 CGO）
- **Admin REST API** -- 独立端口的 Session 认证 JSON 管理接口
- **IP 连接限流** -- 基于 IP 的并发连接数限制，保护 NTRIP 端口
- **优雅停机** -- 按序断开 Source、Client，支持超时强制退出

## 架构概览

```
Internet
   │
   ▼
┌──────────────────────┐
│   NTRIP TCP :2101    │  ← 基站 (SOURCE/POST) & Rover (GET)
│   Caster Core        │
└─────────┬────────────┘
          │ 进程内调用
          ▼
┌──────────────────────┐
│   Admin API :8080    │  ← Web / curl 管理
│   REST JSON          │
└─────────┬────────────┘
          │
          ▼
┌──────────────────────┐
│   SQLite (caster.db) │
└──────────────────────┘
```

单二进制、模块化单体设计。完整架构设计请参阅 [`docs/go_ntrip_caster_architecture.md`](docs/go_ntrip_caster_architecture.md)。

## 快速开始

### 环境要求

- Go 1.22+

### 编译与运行

```bash
# 克隆仓库
git clone https://github.com/lf7817/ntrip-caster.git
cd ntrip-caster

# 编译
go build -o bin/caster ./cmd/caster

# 运行（默认读取当前目录下的 config.yaml）
./bin/caster

# 指定配置文件
./bin/caster -config /path/to/config.yaml
```

首次启动时会自动创建默认管理员账号：

| 用户名   | 密码    | 角色  |
|----------|---------|-------|
| `admin`  | `admin` | admin |

> **生产环境请务必立即修改默认密码。**

## 配置说明

创建或编辑 `config.yaml`：

```yaml
server:
  listen: ":2101"        # NTRIP 端口（基站 + Rover + Sourcetable）
  admin_listen: ":8080"  # Admin REST API 端口

auth:
  enabled: true
  admin_mode: session          # 基于 cookie 的 session 认证
  ntrip_source_auth: user_binding  # user_binding | secret
  ntrip_rover_auth: basic          # none | basic

database:
  type: sqlite
  path: caster.db

limits:
  max_clients: 5000     # 全局 Rover 连接上限
  max_conn_per_ip: 10   # 单 IP 最大并发连接数

mountpoint_defaults:
  write_queue: 64       # 每客户端 WriteChan 缓冲大小
  write_timeout: 3s     # 每客户端写超时
```

## API 接口

除登录外，所有管理接口均需有效的 session cookie。

| 方法     | 路径                      | 说明             |
|----------|---------------------------|------------------|
| `POST`   | `/api/login`              | 管理员登录       |
| `POST`   | `/api/logout`             | 管理员登出       |
| `GET`    | `/api/users`              | 用户列表         |
| `POST`   | `/api/users`              | 创建用户         |
| `PUT`    | `/api/users/{id}`         | 更新用户         |
| `DELETE` | `/api/users/{id}`         | 删除用户         |
| `GET`    | `/api/mountpoints`        | 挂载点列表       |
| `POST`   | `/api/mountpoints`        | 创建挂载点       |
| `PUT`    | `/api/mountpoints/{id}`   | 更新挂载点       |
| `DELETE` | `/api/mountpoints/{id}`   | 删除挂载点       |
| `GET`    | `/api/sources`            | 在线 Source 列表 |
| `GET`    | `/api/clients`            | 在线 Client 列表 |
| `DELETE` | `/api/sources/{mount}`    | 踢除 Source      |
| `DELETE` | `/api/clients/{id}`       | 踢除 Client      |
| `GET`    | `/api/stats`              | 运行时统计       |

### 示例：登录并创建挂载点

```bash
# 登录
curl -c cookies.txt -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'

# 创建挂载点
curl -b cookies.txt -X POST http://localhost:8080/api/mountpoints \
  -H 'Content-Type: application/json' \
  -d '{"name":"RTCM_01","description":"基站 01","format":"RTCM3"}'

# 查看统计信息
curl -b cookies.txt http://localhost:8080/api/stats
```

## 项目结构

```
ntrip-caster/
├── cmd/caster/              # 程序入口
│   └── main.go
├── internal/
│   ├── api/                 # Admin REST API（handlers、session 中间件）
│   ├── account/             # 用户与挂载点持久化（SQLite）
│   ├── auth/                # HTTP Basic Auth 解析
│   ├── caster/              # NTRIP TCP 服务器与协议解析
│   ├── client/              # Rover 客户端写循环
│   ├── config/              # YAML 配置加载
│   ├── database/            # SQLite 初始化与 schema
│   ├── limiter/             # IP 连接限流器
│   ├── metrics/             # 原子计数器（per-mountpoint 统计）
│   ├── mountpoint/          # 挂载点管理与 fanout 广播
│   ├── rtcm/                # RTCM3 帧解析器与包类型
│   ├── source/              # 基站读循环
│   └── sourcetable/         # NTRIP Sourcetable 生成器
├── pkg/logger/              # 结构化日志（slog）
├── docs/                    # 架构与设计文档
├── config.yaml              # 默认配置文件
└── go.mod
```

## 性能目标

| 指标           | 目标                                |
|----------------|-------------------------------------|
| Rover 连接数   | 单核 2,000 -- 5,000                 |
| 广播延迟       | < 50 ms（不含公网抖动）             |
| 内存占用       | 5,000 并发客户端约 4 GB             |

> 以上为工程目标，上线前务必使用真实 RTCM 流量进行压测验证。

## 路线图

- [ ] NTRIP TLS 加密
- [ ] Prometheus 指标端点
- [ ] Web 管理界面
- [ ] 集群模式（多节点）
- [ ] Rover 地图可视化
- [ ] 基站自动注册
- [ ] 流量统计与计费

## 参与贡献

欢迎贡献代码！请先开 Issue 讨论你想要做的改动。

1. Fork 本仓库
2. 创建功能分支（`git checkout -b feature/my-feature`）
3. 提交改动（`git commit -m 'feat: 添加某功能'`）
4. 推送分支（`git push origin feature/my-feature`）
5. 发起 Pull Request

提交前请确保：
- 代码通过 `go build ./...`、`go vet ./...` 和 `go test ./...`
- 导出符号包含 godoc 注释
- 并发代码遵守[架构约束](docs/go_ntrip_caster_architecture.md)

## 开源协议

本项目基于 [MIT 协议](LICENSE) 开源。
