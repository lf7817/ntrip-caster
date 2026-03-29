# Nginx TLS 反向代理配置

通过 Nginx 反向代理实现 NTRIP Caster 的 TLS 加密，无需修改 Caster 代码。

## 架构

```
┌─────────────┐     TLS/443      ┌─────────────────┐     明文 TCP     ┌─────────────┐
│  Rover/Base │ ──────────────▶  │  Nginx          │ ─────────────▶  │   NTRIP     │
│  (TLS 连接)  │                  │  (TLS 终结)      │    :2101       │   Caster    │
└─────────────┘                  └─────────────────┘                 └─────────────┘
                                       │
                                       │ 证书管理
                                       ▼
                                 SSL 证书文件
```

## Nginx 配置

Nginx 需要 `stream` 模块（TCP 代理），配置放在 `nginx.conf` 主文件中（在 `http` 块之外）：

```nginx
# /etc/nginx/nginx.conf

# TCP Stream 代理（NTRIP TLS）
stream {
    upstream ntrip_backend {
        server 127.0.0.1:2101;
    }

    server {
        listen 2102 ssl;  # TLS NTRIP 端口
        ssl_certificate     /path/to/your/domain.crt;
        ssl_certificate_key /path/to/your/domain.key;
        ssl_protocols       TLSv1.2 TLSv1.3;
        ssl_ciphers         HIGH:!aNULL:!MD5;

        proxy_pass ntrip_backend;
        proxy_timeout 1h;           # 长连接超时，NTRIP 连接持续时间长
        proxy_connect_timeout 10s;
    }
}

# HTTP 代理（Admin API TLS）
http {
    # ... 其他配置 ...

    upstream admin_backend {
        server 127.0.0.1:8080;
    }

    server {
        listen 8443 ssl;
        server_name your-domain.com;

        ssl_certificate     /path/to/your/domain.crt;
        ssl_certificate_key /path/to/your/domain.key;
        ssl_protocols       TLSv1.2 TLSv1.3;

        location / {
            proxy_pass http://admin_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }
}
```

## 配置要点

| 参数 | 说明 |
|------|------|
| `proxy_timeout 1h` | NTRIP 是长连接，需要设置足够长的超时 |
| `stream {}` 块 | 必须在 `http {}` 块之外，与 http 同级 |
| `listen 2102 ssl` | 自定义 TLS 端口，客户端连接此端口 |
| `proxy_connect_timeout` | 连接后端的超时，通常很短 |

## 客户端使用方式

```
明文连接：your-ip:2101        （不经过 Nginx）
TLS 连接：your-domain:2102    （通过 Nginx TLS）
```

## 检查 Nginx 是否支持 stream 模块

```bash
nginx -V 2>&1 | grep -o with-stream
# 输出 "with-stream" 表示支持
```

如果不支持，需要重新编译或安装完整版本：

```bash
# Ubuntu/Debian
apt install libnginx-mod-stream

# 或使用完整包
apt install nginx-full
```

## 优势

- Caster 代码零改动
- 证书管理集中在反向代理层
- 可复用现有的 SSL 证书基础设施
- 便于统一管理多个服务的 TLS

## 注意事项

- 多一层代理，增加微小延迟（通常 <1ms，可忽略）
- 反向代理需要正确处理 TCP 长连接
- 客户端需要配置连接到反向代理的 TLS 端口