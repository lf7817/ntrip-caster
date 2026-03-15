# NTRIP Caster 测试指南

本文档覆盖功能验证和压力测试的完整流程。项目提供以下测试工具：

| 工具 | 用途 |
|------|------|
| `cmd/testenv` | 一键初始化测试环境（配置 + 数据库 + 账号 + 挂载点） |
| `cmd/simbase` | 模拟基站（Source），向 caster 推送合成 RTCM3 帧 |
| `cmd/simrover` | 模拟流动站（Rover），从 caster 接收 RTCM3 数据 |

---

## 1. 一键搭建测试环境

**不需要手动改配置、不需要调 API、不需要创建账号。** `testenv` 一条命令搞定一切。

### 1.1 免认证环境（最快上手）

```bash
make test-env
```

自动完成：
- 生成 `config_test.yaml`（关闭认证、放开连接限制）
- 创建 `caster_test.db`（独立数据库，不影响生产数据）
- 创建挂载点 `BENCH`
- 创建账号 `admin/admin`、`base/test`、`rover1/test`

输出示例：

```
✓ config written to config_test.yaml
✓ users: admin/admin, base/test (base), rover1/test (rover)
✓ mountpoints: BENCH

═══════════════════════════════════════════════
  Test environment ready!
═══════════════════════════════════════════════

  Quick start:

    make test-caster

    go run ./cmd/simbase  -mount BENCH
    go run ./cmd/simrover -mount BENCH
```

### 1.2 带认证 + 多挂载点环境

```bash
make test-env-auth
```

等价于 `go run ./cmd/testenv -mounts 5 -auth`，自动完成：
- 启用认证（`user_binding` + `basic`）
- 创建 5 个挂载点：`BENCH_0` ~ `BENCH_4`
- `base` 用户绑定到全部 5 个挂载点（publish 权限）
- `rover1` 用户绑定到全部 5 个挂载点（subscribe 权限）

### 1.3 自定义环境

```bash
# 10 个挂载点，自定义前缀，带认证
go run ./cmd/testenv -mounts 10 -prefix RTCM -auth

# 调整队列参数
go run ./cmd/testenv -mounts 3 -write-queue 256 -write-timeout 10s
```

`testenv` 完整参数：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mounts` | `1` | 挂载点数量 |
| `-prefix` | `BENCH` | 挂载点名称前缀 |
| `-auth` | `false` | 是否启用认证 |
| `-max-clients` | `10000` | 最大 rover 数 |
| `-max-conn-per-ip` | `12000` | 单 IP 最大连接数 |
| `-write-queue` | `128` | 每挂载点写队列大小 |
| `-write-timeout` | `5s` | 写超时 |

### 1.4 清理测试环境

```bash
make test-clean
```

---

## 2. 功能测试

### 2.1 基础功能验证（免认证）

```bash
# 终端 1：搭建环境 + 启动 caster
make test-env
make test-caster

# 终端 2：启动模拟基站
go run ./cmd/simbase -mount BENCH -interval 1s

# 终端 3：启动模拟流动站
go run ./cmd/simrover -mount BENCH
```

**预期结果：**

- simbase 每秒输出 `[stats] N pkts, M bytes, ...`
- simrover 逐帧打印 `RTCM msg=1005 len=25`
- caster 日志显示 `source connected` 和 `client connected`

### 2.2 带认证的功能测试

```bash
# 终端 1：搭建环境 + 启动 caster
make test-env-auth
make test-caster

# 终端 2：base（Rev2 + Basic Auth）
go run ./cmd/simbase -mount BENCH_0 -user base -pass test

# 终端 3：rover（Basic Auth）
go run ./cmd/simrover -mount BENCH_0 -user rover1 -pass test
```

**验证要点：**

- 错误凭证 → 打印 `rejected: ... 401 Unauthorized`
- 无绑定的用户 → 打印 `rejected: ... 403 Forbidden`
- 正确凭证 → 正常推拉数据

### 2.3 NTRIP Rev1 协议测试

```bash
# 免认证环境下
go run ./cmd/simbase -mount BENCH -pass secret123 -rev 1
```

---

## 3. 压力测试

### 3.1 场景一：1 Base + 5000 Rovers

测试单挂载点的广播扇出能力。

```bash
# 终端 1
make test-env && make test-caster

# 终端 2
go run ./cmd/simbase -mount BENCH -interval 100ms -size 200

# 终端 3
go run ./cmd/simrover -mount BENCH -count 5000 -ramp 2ms
```

> 也可以用一键命令（需要先在终端 1 运行 `make test-env && make test-caster`）：
> ```bash
> make test-bench-1b
> ```

**simrover 输出：**

```
[aggregate] connected=5000 failed=0 disconnected=0 kicked=0 | pkts=48230 bytes=9929380 rate=1938.5 KB/s | 5s
```

| 指标 | 含义 | 关注点 |
|------|------|--------|
| `connected` | 成功连接的 rover 数 | 应等于 count |
| `failed` | 连接失败数 | 应为 0 |
| `kicked` | 被踢慢客户端数 | 大于 0 说明需调优 `write_queue` / `write_timeout` |
| `rate` | 全部 rover 的总接收速率 | 吞吐量上限 |

### 3.2 场景二：5 Bases + 5000 Rovers

测试多挂载点并行服务的能力。每个挂载点 1 base + 1000 rovers。

```bash
# 终端 1：5 个挂载点的测试环境
go run ./cmd/testenv -mounts 5
make test-caster

# 终端 2：5 个 base 同时推流
go run ./cmd/simbase -count 5 -mount-prefix BENCH -interval 100ms -size 200

# 终端 3：5000 rover 均匀分配到 5 个挂载点
go run ./cmd/simrover -mounts BENCH_0,BENCH_1,BENCH_2,BENCH_3,BENCH_4 -count 5000 -ramp 2ms
```

> 一键命令（需先在终端 1 启动 caster）：
> ```bash
> make test-bench-mb
> ```

### 3.3 场景三：大帧吞吐量测试

最大 RTCM3 帧（1029 字节）下的吞吐极限。

```bash
go run ./cmd/simbase -mount BENCH -interval 50ms -size 1023
go run ./cmd/simrover -mount BENCH -count 2000 -ramp 2ms
```

### 3.4 场景四：慢客户端踢除测试

```bash
# 小队列 + 高频推送，故意制造慢客户端
go run ./cmd/testenv -write-queue 4 -write-timeout 500ms
make test-caster

go run ./cmd/simbase -mount BENCH -interval 10ms -size 500
go run ./cmd/simrover -mount BENCH -count 1000 -ramp 1ms
```

**预期：** simrover 的 `kicked` 指标持续增长。

---

## 4. 完整操作速查

### 最快上手（3 条命令）

```bash
make test-env          # 搭建环境
make test-caster       # 启动 caster（终端 1）
# 分别在终端 2、3 运行：
go run ./cmd/simbase -mount BENCH
go run ./cmd/simrover -mount BENCH
```

### 1 Base + 5000 Rovers 压测（4 条命令）

```bash
make test-env          # 搭建环境
make test-caster       # 终端 1
go run ./cmd/simbase -mount BENCH -interval 100ms -size 200               # 终端 2
go run ./cmd/simrover -mount BENCH -count 5000 -ramp 2ms                  # 终端 3
```

### 5 Bases + 5000 Rovers 压测（4 条命令）

```bash
go run ./cmd/testenv -mounts 5                                            # 搭建环境
make test-caster                                                          # 终端 1
go run ./cmd/simbase -count 5 -mount-prefix BENCH -interval 100ms         # 终端 2
go run ./cmd/simrover -mounts BENCH_0,BENCH_1,BENCH_2,BENCH_3,BENCH_4 -count 5000 -ramp 2ms  # 终端 3
```

### 带认证的完整链路压测（4 条命令）

```bash
go run ./cmd/testenv -mounts 5 -auth                                      # 搭建环境
make test-caster                                                          # 终端 1
go run ./cmd/simbase -count 5 -mount-prefix BENCH -user base -pass test -interval 100ms      # 终端 2
go run ./cmd/simrover -mounts BENCH_0,BENCH_1,BENCH_2,BENCH_3,BENCH_4 -count 5000 -ramp 2ms -user rover1 -pass test  # 终端 3
```

---

## 5. Makefile 目标速查

| 目标 | 说明 |
|------|------|
| `make test-env` | 1 挂载点、免认证的测试环境 |
| `make test-env-auth` | 5 挂载点、带认证的测试环境 |
| `make test-caster` | 用 `config_test.yaml` 启动 caster |
| `make test-bench-1b` | 1 base + 5000 rovers 压测 |
| `make test-bench-mb` | 5 bases + 5000 rovers 压测 |
| `make test-clean` | 清理测试配置和数据库 |
| `make simbase ARGS="..."` | 运行 simbase |
| `make simrover ARGS="..."` | 运行 simrover |

---

## 6. simbase 参数参考

```
  -addr string         caster 地址 (default "127.0.0.1:2101")
  -mount string        挂载点名称（单 base 模式）
  -mount-prefix string 挂载点前缀（多 base 模式） (default "BENCH")
  -count int           base 数量，每个连接独立的挂载点 (default 1)
  -user string         Basic Auth 用户名（Rev2）
  -pass string         密码
  -rev int             NTRIP 版本：1 或 2 (default 2)
  -interval duration   RTCM 帧发送间隔 (default 1s)
  -msgtype int         RTCM3 消息类型号 (default 1005)
  -size int            RTCM3 载荷字节数 (default 19)
```

| 模式 | 参数 | 挂载点 |
|------|------|--------|
| 单 base | `-mount TEST` | `TEST` |
| 多 base | `-count 3 -mount-prefix MP` | `MP_0`, `MP_1`, `MP_2` |

---

## 7. simrover 参数参考

```
  -addr string     caster 地址 (default "127.0.0.1:2101")
  -mount string    单挂载点名称
  -mounts string   逗号分隔的挂载点列表（rover 按 round-robin 分配）
  -count int       并发 rover 数量 (default 1)
  -ramp duration   每个连接之间的间隔 (default 5ms)
  -quiet           静默模式（count > 10 自动开启）
  -user string     Basic Auth 用户名
  -pass string     密码
```

| 模式 | 参数 | 分配 |
|------|------|------|
| 单挂载点 | `-mount TEST -count 100` | 100 rover → `TEST` |
| 多挂载点 | `-mounts A,B,C -count 90` | 30 rover × 3 mounts |

---

## 8. 监控与观测

### 8.1 Admin API 实时统计

Admin API 需要先登录。压测期间可在另一个终端持续观察：

```bash
# 登录
curl -c cookies.txt -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'

# 查看全局统计
curl -s -b cookies.txt http://localhost:8080/api/stats | jq

# 查看在线 source / client
curl -s -b cookies.txt http://localhost:8080/api/sources | jq
curl -s -b cookies.txt http://localhost:8080/api/clients | jq
```

Stats API 输出示例：

```json
{
  "total_clients": 5000,
  "total_sources": 1,
  "mountpoints": [
    {
      "name": "BENCH",
      "client_count": 5000,
      "source_online": true,
      "bytes_in": 246994,
      "bytes_out": 971773482,
      "slow_clients": 0,
      "kick_count": 0
    }
  ]
}
```

可用 `watch` 持续刷新：

```bash
watch -n 2 'curl -s -b cookies.txt http://localhost:8080/api/stats | jq'
```

### 8.2 查看 TCP 连接数

```bash
# 统计到 caster 端口 2101 的连接总数
netstat -an | grep 2101 | wc -l
```

> 连接数 ≈ base 数 + rover 数 + LISTEN × 2（TCP 双端），5000 rovers 时约 10000+。

### 8.3 查看 caster 进程性能（macOS）

> **注意：** `pgrep caster` 可能匹配到 Cursor IDE 等无关进程。需先找到正确的 PID。

```bash
# 方法一：通过端口找 PID（推荐）
lsof -iTCP:2101 -sTCP:LISTEN -t
# 输出单个 PID，如 76589

# 方法二：通过命令行特征找 PID
ps aux | grep '[c]md/caster'
# 或
pgrep -f 'cmd/caster'
```

拿到 PID 后：

```bash
# 设置变量方便后续使用
CASTER_PID=$(lsof -iTCP:2101 -sTCP:LISTEN -t)

# 查看打开的文件描述符数量
lsof -p $CASTER_PID | wc -l

# 实时查看 CPU / 内存（macOS）
top -l 0 -pid $CASTER_PID -stats pid,cpu,mem,threads,ports

# 也可以用 Activity Monitor 搜索 caster
```

### 8.4 查看 caster 进程性能（Linux）

```bash
CASTER_PID=$(lsof -iTCP:2101 -sTCP:LISTEN -t)

# fd 数量
ls /proc/$CASTER_PID/fd | wc -l

# 实时 CPU / 内存
top -p $CASTER_PID

# 或用 htop
htop -p $CASTER_PID
```

### 8.5 压测结果解读示例

以实际跑出的 1 Base + 5000 Rovers 压测为例：

```
simbase:
  [aggregate] pkts=1199 bytes=246994 rate=2.0 KB/s | 2m0s

simrover:
  [aggregate] connected=5000 failed=0 disconnected=0 kicked=0 | pkts=4717347 bytes=971773482 rate=9489.9 KB/s | 1m40s
```

| 指标 | 值 | 分析 |
|------|-----|------|
| simbase rate | 2.0 KB/s | 100ms 间隔 × 206 字节/帧 ≈ 2 KB/s，符合预期 |
| rover connected | 5000 | 全部连接成功 |
| rover failed | 0 | 无连接失败 |
| rover kicked | 0 | 无慢客户端，所有 rover 跟得上 |
| rover rate | ~9.5 MB/s | 5000 rovers × 2 KB/s = ~10 MB/s（理论值），实际 9.5 MB/s 接近理论值 |
| rover pkts | 4,717,347 | ≈ 1199 帧 × 5000 rovers × (100s/120s)，考虑 ramp-up 延迟，合理 |
| netstat 连接数 | 10013 | ≈ 1 base + 5000 rovers + 5000 对端 + LISTEN + 少量其他，正常 |
| lsof fd | 5021 | ≈ 5000 client conn + base conn + 标准 fd，正常 |
| caster CPU | ~60% | 单核跑满（Go runtime 调度），生产环境可多核利用 |

**关键结论：** caster 在单机 MacBook 上稳定支撑 5000 并发 rover、10 Hz 推送、零丢包。

---

## 9. 压测调优参考

| 参数 | 调大 | 调小 |
|------|------|------|
| `write_queue` | 减少踢除，内存增加 | 更快踢除慢客户端 |
| `write_timeout` | 容忍网络延迟 | 快速释放僵死连接 |
| `max_clients` | 更多并发 rover | 保护服务器资源 |
| `max_conn_per_ip` | 同 IP 更多连接（压测必须） | 防 IP 攻击 |
| simbase `-interval` | 降低帧率 | 测吞吐极限 |
| simbase `-size` | 测带宽瓶颈 | 测 pps 上限 |
| simrover `-ramp` | 缓慢建连 | 测连接风暴 |

---

## 10. 常见问题

### Q: simrover 大量 `failed`

- **`max_conn_per_ip`**：同机器压测需大于总连接数。用 `make test-env` 自动设置为 12000。
- **fd 限制**：`ulimit -n` 需大于 caster + simrover 总连接数。
- **端口耗尽**：macOS `sysctl net.inet.ip.portrange.first`，Linux `sysctl net.ipv4.ip_local_port_range`。

### Q: `kicked` 持续增长

rover 跟不上 base 推送速度：
1. 增大 `write_queue`（128 → 256）
2. 增大 `write_timeout`（5s → 10s）
3. 降低 simbase `-interval`

### Q: `pgrep caster` 匹配到多个进程

在 Cursor IDE 中开发时，`pgrep caster` 会匹配到 IDE 的 helper 进程。正确找 caster PID 的方式：

```bash
# 通过监听端口精确定位
lsof -iTCP:2101 -sTCP:LISTEN -t

# 或通过命令行特征
pgrep -f 'cmd/caster'
```

### Q: macOS 上 `top -pid` 报错

macOS 的 `top` 不支持传多个 PID。用以下方式：

```bash
# 用 -pid 传单个 PID
top -l 0 -pid $(lsof -iTCP:2101 -sTCP:LISTEN -t) -stats pid,cpu,mem,threads

# 或直接用 ps 看瞬时值
ps -p $(lsof -iTCP:2101 -sTCP:LISTEN -t) -o pid,%cpu,%mem,rss,vsz,command
```

### Q: 测试环境影响生产数据吗

不会。`testenv` 使用独立的 `caster_test.db`，与生产 `caster.db` 互不干扰。
