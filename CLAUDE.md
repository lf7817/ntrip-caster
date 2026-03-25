# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

NTRIP Caster is a production-grade server for relaying RTCM differential correction data from GNSS base stations to rover clients. It implements NTRIP Rev1/Rev2 protocols with high-performance, lock-free broadcasting.

## Build Commands

```bash
# Development
make dev-backend          # Run Go backend (reads config.yaml)
make dev-frontend         # Run Vite dev server on :5173 with API proxy

# Build
make build                # Build frontend + backend binaries
make build-linux-amd64    # Cross-compile for Linux deployment

# Testing & Benchmarking
make test-env             # Initialize test DB with 1 mountpoint (no auth)
make test-env-auth        # Initialize test DB with 5 mountpoints + auth
make test-caster          # Run caster with test config
make test-bench-1b        # Benchmark: 1 base + 5000 rovers
make test-bench-mb        # Benchmark: 5 bases + 5000 rovers
```

## Architecture

```
┌─────────────────────┐
│  NTRIP TCP :2101    │  ← Base (SOURCE/POST) & Rover (GET)
│  caster.Server      │
└──────────┬──────────┘
           │ in-process
           ▼
┌─────────────────────┐
│  Admin API :8080    │  ← REST + embedded web UI
│  api.HTTPServer     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  SQLite (caster.db) │
│  account.Service    │
└─────────────────────┘
```

### Key Packages

| Package | Responsibility |
|---------|----------------|
| `internal/caster` | NTRIP TCP server, protocol parsing, connection handling |
| `internal/mountpoint` | Mountpoint registry, atomic-snapshot broadcast to clients |
| `internal/client` | Rover write loop, slow-client kick mechanism |
| `internal/source` | Base station read loop, RTCM framing |
| `internal/api` | REST handlers, session middleware, SPA fallback |
| `internal/account` | User/mountpoint CRUD, bcrypt auth, SQLite persistence |
| `internal/rtcm` | RTCM3 frame parser (preamble detection, length extraction) |
| `internal/limiter` | Per-IP connection rate limiting |

### Critical Pattern: Atomic Snapshot Broadcast

The `mountpoint.MountPoint` uses `atomic.Value` to store a snapshot of `[]*client.Client`. Writers rebuild the snapshot under lock; readers (Broadcast) never block:

```go
// Broadcast path - lock-free
func (m *MountPoint) Broadcast(pkt *rtcm.RTCMPacket) {
    clients := m.snapshot.Load().([]*client.Client)
    for _, c := range clients {
        select {
        case c.WriteChan <- pkt:
        default:
            c.MarkSlowAndKick()  // CAS-guarded, kicks once
        }
    }
}
```

When modifying broadcast logic, maintain this lock-free read path.

## Frontend (web/)

- **Stack**: React 19 + Vite 8 + shadcn/ui + Tailwind CSS v4 + TanStack Query
- **Build output**: `internal/web/dist/` (embedded via `go:embed`)
- **Dev proxy**: Vite proxies `/api/*` to `localhost:8080`

Frontend routes:
- `/login` - Admin login
- `/` - Dashboard (stats, mountpoint overview)
- `/users` - User management
- `/mountpoints` - Mountpoint management
- `/connections` - Online source/client monitoring

## Configuration

Config file: `config.yaml` (or `-config` flag)

Key settings:
```yaml
server:
  listen: ":2101"        # NTRIP port
  admin_listen: ":8080"  # Admin API

auth:
  enabled: true
  admin_mode: session           # cookie-based auth
  ntrip_source_auth: user_binding  # or "secret"
  ntrip_rover_auth: basic       # or "none"

limits:
  max_clients: 5000      # global rover cap
  max_conn_per_ip: 10    # per-IP rate limit
```

## Database Schema

SQLite tables (see `internal/database/database.go`):
- `users` - id, username, password_hash, role (admin/base/rover), enabled
- `mountpoints` - id, name, description, format, enabled, write_queue, write_timeout_ms, secret
- `user_mountpoint_bindings` - user_id, mountpoint_id, permission (publish/subscribe)

Default admin created on first run: `admin`/`admin`

## Simulator Tools

| Tool | Purpose |
|------|---------|
| `cmd/simbase` | Simulates NTRIP base station pushing RTCM data |
| `cmd/simrover` | Simulates NTRIP rover clients receiving data |
| `cmd/testenv` | One-command test environment setup |

Example:
```bash
# Terminal 1: Start caster
make test-env && make test-caster

# Terminal 2: Simulate base
go run ./cmd/simbase -mount MOUNT01 -interval 100ms -size 200

# Terminal 3: Simulate rovers
go run ./cmd/simrover -mount MOUNT01 -count 100
```

## Adding New API Endpoints

1. Add handler in `internal/api/handlers.go`
2. Register route in `internal/api/http_server.go`
3. Add types to `web/src/api/types.ts`
4. Add API client method in `web/src/api/client.ts`
5. Add TanStack Query hook in `web/src/api/hooks.ts`

## Code Style Notes

- Go: standard Go style, slog for logging
- Concurrency: prefer channels over shared memory; use `sync/atomic` for counters
- Error handling: log at source, return errors up the call stack
- Frontend: functional components, hooks for state, TanStack Query for server state