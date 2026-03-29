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

## 部署到 Linux amd64

在本地（如 macOS/Windows）交叉编译出 Linux 可执行文件，再上传到服务器运行。

### 1. 编译 Linux amd64 二进制

需先安装 [Bun](https://bun.sh) 以构建前端（前端会嵌入到 caster 二进制中）：

```bash
# 一键：先构建前端，再交叉编译所有 Linux amd64 二进制
make build-linux-amd64
```

产物在 `bin/` 下：

- `bin/caster-linux-amd64` — 主服务（NTRIP + Admin API）
- `bin/simbase-linux-amd64`、`bin/simrover-linux-amd64`、`bin/testenv-linux-amd64` — 测试/压测工具

若只想要 caster，可手动执行：

```bash
make build-frontend
GOOS=linux GOARCH=amd64 go build -o bin/caster-linux-amd64 ./cmd/caster
```

### 2. 上传与运行

将 `caster-linux-amd64` 和 `config.yaml` 拷到 Linux 服务器，例如：

```bash
scp bin/caster-linux-amd64 user@your-server:/opt/ntrip-caster/caster
scp config.yaml user@your-server:/opt/ntrip-caster/
```

在服务器上可直接运行：

```bash
chmod +x /opt/ntrip-caster/caster
cd /opt/ntrip-caster
./caster -config config.yaml
```

### 3. 使用 systemd 管理（推荐）

使用仓库内提供的 unit 文件，可开机自启、自动重启与日志统一到 journald。

**安装步骤：**

```bash
# 1. 创建专用用户（可选，提高安全性）
sudo useradd -r -s /usr/sbin/nologin ntrip-caster

# 2. 确保目录与权限
sudo mkdir -p /opt/ntrip-caster
sudo cp /path/to/caster-linux-amd64 /opt/ntrip-caster/caster
sudo cp /path/to/config.yaml /opt/ntrip-caster/
sudo chmod +x /opt/ntrip-caster/caster
sudo chown -R ntrip-caster:ntrip-caster /opt/ntrip-caster

# 3. 安装并启用 systemd 服务
sudo cp deploy/ntrip-caster.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable ntrip-caster
sudo systemctl start ntrip-caster
sudo systemctl status ntrip-caster
```

**常用命令：**

| 命令 | 说明 |
|------|------|
| `sudo systemctl start ntrip-caster` | 启动 |
| `sudo systemctl stop ntrip-caster` | 停止 |
| `sudo systemctl restart ntrip-caster` | 重启 |
| `sudo systemctl status ntrip-caster` | 状态 |
| `journalctl -u ntrip-caster -f` | 实时查看日志 |

若使用 root 运行，将 unit 中的 `User=` 与 `Group=` 改为 `root`，或删除这两行即可。

### 4. 使用 Docker 部署（Ubuntu 24.04 镜像）

镜像为多阶段构建：在容器内编译前端与 Go 二进制，最终运行镜像是 **Ubuntu 24.04**，仅包含单个可执行文件与 CA 证书。

**仅用 Docker 运行：**

```bash
# 构建镜像
docker build -t ntrip-caster:latest .

# 运行（使用镜像内默认 config.yaml，数据不持久化）
docker run -d --name ntrip-caster -p 2101:2101 -p 8080:8080 ntrip-caster:latest
```

**使用 Docker Compose（推荐，便于挂载配置与持久化数据库）：**

```bash
# 使用 Docker 专用配置（database.path 指向 /app/data/caster.db）
cp deploy/config-docker.yaml config.yaml
# 按需编辑 config.yaml

docker compose up -d
```

| 说明       | 做法 |
|------------|------|
| 自定义配置 | 将 `config.yaml` 放在项目根目录，compose 会挂载为 `/app/config.yaml` |
| 持久化数据库 | 使用 `deploy/config-docker.yaml` 作为 `config.yaml`（内含 `path: /app/data/caster.db`），compose 已挂载命名卷 `ntrip-caster-data` 到 `/app/data` |
| 查看日志   | `docker compose logs -f ntrip-caster` |
| 重启       | `docker compose restart ntrip-caster` |

部署时请放行 NTRIP 端口（默认 2101）和 Admin 端口（默认 8080）。

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
├── cmd/
│   ├── caster/              # 主程序入口
│   ├── simbase/             # 模拟基站（压测工具）
│   ├── simrover/            # 模拟流动站（压测工具）
│   └── testenv/             # 一键初始化测试环境
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

## 测试与压测

项目内置了完整的模拟工具链，无需真实 GNSS 硬件即可进行端到端测试和压力测试。

### 一键搭建测试环境

```bash
make test-env       # 生成测试配置 + 数据库 + 账号 + 挂载点
make test-caster    # 启动 caster（使用测试配置）
```

### 功能验证

```bash
go run ./cmd/simbase  -mount BENCH              # 终端 2：模拟基站推流
go run ./cmd/simrover -mount BENCH              # 终端 3：模拟 rover 收流
```

### 压力测试

```bash
# 1 Base + 5000 Rovers
go run ./cmd/simbase  -mount BENCH -interval 100ms -size 200
go run ./cmd/simrover -mount BENCH -count 5000 -ramp 2ms

# 5 Bases + 5000 Rovers（多挂载点）
go run ./cmd/testenv -mounts 5 && make test-caster
go run ./cmd/simbase  -count 5 -mount-prefix BENCH -interval 100ms
go run ./cmd/simrover -mounts BENCH_0,BENCH_1,BENCH_2,BENCH_3,BENCH_4 -count 5000 -ramp 2ms
```

完整的测试步骤、参数说明、监控方法和调优指引详见 [`docs/testing.md`](docs/testing.md)。

## 性能目标

| 指标           | 目标                                |
|----------------|-------------------------------------|
| Rover 连接数   | 单核 2,000 -- 5,000                 |
| 广播延迟       | < 50 ms（不含公网抖动）             |
| 内存占用       | 5,000 并发客户端约 4 GB             |

> 以上为工程目标，上线前务必使用真实 RTCM 流量进行压测验证。

## 路线图

### 已完成

- [x] Web 管理界面（React SPA）
- [x] 流量统计（实时 BytesIn/BytesOut）
- [x] 用户/挂载点管理（CRUD + 角色权限）
- [x] 慢客户端踢除（CAS 防抖 + 写队列）
- [x] IP 限流（单 IP 并发连接数限制）
- [x] 优雅停机（按序断开 Source/Client）

### 待实现

| 优先级 | 功能 | 说明 | 设计文档 |
|--------|------|------|----------|
| P0 | Prometheus `/metrics` | 运维监控必备 | - |
| P1 | 基站位置解析 (1005) | 解析 RTCM 1005 提取基站坐标，地图展示 | [设计文档](docs/superpowers/specs/2026-03-29-base-station-position-design.md) |
| P2 | 流量历史记录 | SQLite 表记录每分钟/每小时流量，支持趋势图 | - |
| P3 | 集群模式 | 需重构架构，引入 Redis/Etcd 做 mountpoint 同步 | - |

### TLS 加密方案

NTRIP Caster 通过 **Nginx 反向代理** 实现 TLS 加密，无需修改 Caster 代码：

- Nginx 在 `stream {}` 块配置 TCP 代理，终结 TLS 后转发至 Caster 明文端口
- Admin API 通过 HTTP 代理实现 TLS
- 证书管理集中在 Nginx 层，便于运维

详细配置请参阅 [`docs/nginx_tls_proxy.md`](docs/nginx_tls_proxy.md)。

### 移除

- 基站自动注册 — 不需要，手动管理即可
- 计费功能 — 小规模部署暂不需要

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
