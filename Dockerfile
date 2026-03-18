# -----------------------------------------------------------------------------
# Stage 1: 构建前端 + Go 二进制
# -----------------------------------------------------------------------------
FROM golang:1.26-bookworm AS builder

# 从官方镜像复制 Bun，避免构建时从 GitHub 下载（易遇网络/HTTP2 错误）
COPY --from=oven/bun:1 /usr/local/bin/bun /usr/local/bin/bun

WORKDIR /build

# 依赖先复制并下载，便于利用层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 构建前端到 internal/web/dist（供 embed 使用）
RUN cd web && bun install --frozen-lockfile && bun run build

# 编译 Linux amd64 二进制（无 CGO，静态链接）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /caster ./cmd/caster

# -----------------------------------------------------------------------------
# Stage 2: 运行镜像 Ubuntu 24.04 精简（仅 CA 证书，满足运行即可）
# -----------------------------------------------------------------------------
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# 使用非 root 用户运行（不指定 -u，由系统分配唯一 UID，避免与基础镜像已有 UID 冲突）
RUN useradd -r -s /usr/sbin/nologin ntrip-caster

WORKDIR /app

# 从 builder 阶段复制二进制
COPY --from=builder /caster /app/caster

# 默认配置（可被挂载的 config.yaml 覆盖）
COPY config.yaml /app/config.yaml

RUN chown -R ntrip-caster:ntrip-caster /app

USER ntrip-caster

# NTRIP 端口 / Admin API 端口
EXPOSE 2101 8080

ENTRYPOINT ["/app/caster"]
CMD ["-config", "/app/config.yaml"]
