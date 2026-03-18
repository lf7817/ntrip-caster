.PHONY: dev-frontend dev-backend build build-frontend build-backend clean \
       build-linux build-linux-amd64 \
       simbase simrover test-env test-env-auth test-caster test-bench-1b test-bench-mb

# ─── Development ───────────────────────────────────────────────

dev-frontend:
	cd web && bun run dev

dev-backend:
	go run ./cmd/caster

# ─── Build ─────────────────────────────────────────────────────

build: build-frontend build-backend

build-frontend:
	cd web && bun run build

build-backend:
	go build -o bin/caster ./cmd/caster
	go build -o bin/simbase ./cmd/simbase
	go build -o bin/simrover ./cmd/simrover
	go build -o bin/testenv ./cmd/testenv

# 交叉编译：Linux amd64（用于部署到服务器）
build-linux-amd64: build-frontend
	GOOS=linux GOARCH=amd64 go build -o bin/caster-linux-amd64 ./cmd/caster
	GOOS=linux GOARCH=amd64 go build -o bin/simbase-linux-amd64 ./cmd/simbase
	GOOS=linux GOARCH=amd64 go build -o bin/simrover-linux-amd64 ./cmd/simrover
	GOOS=linux GOARCH=amd64 go build -o bin/testenv-linux-amd64 ./cmd/testenv

# 同上，简短别名
build-linux: build-linux-amd64

# ─── Simulator ─────────────────────────────────────────────────

simbase:
	go run ./cmd/simbase $(ARGS)

simrover:
	go run ./cmd/simrover $(ARGS)

# ─── Test Environment ─────────────────────────────────────────

# 一键搭建测试环境（1 挂载点，免认证）
test-env:
	go run ./cmd/testenv

# 一键搭建测试环境（5 挂载点，带认证）
test-env-auth:
	go run ./cmd/testenv -mounts 5 -auth

# 启动 caster（使用测试配置）
test-caster:
	go run ./cmd/caster -config config_test.yaml

# 一键压测：1 base + 5000 rover（先 make test-env）
test-bench-1b:
	@echo "=== Starting simbase (background) ==="
	@go run ./cmd/simbase -mount BENCH -interval 100ms -size 200 &
	@sleep 2
	@echo "=== Starting 5000 rovers ==="
	@go run ./cmd/simrover -mount BENCH -count 5000 -ramp 2ms

# 一键压测：5 bases + 5000 rovers（先 make test-env-auth 或 make test-env -mounts 5）
test-bench-mb:
	@echo "=== Starting 5 simbase (background) ==="
	@go run ./cmd/simbase -count 5 -mount-prefix BENCH -interval 100ms -size 200 &
	@sleep 2
	@echo "=== Starting 5000 rovers across 5 mounts ==="
	@go run ./cmd/simrover -mounts BENCH_0,BENCH_1,BENCH_2,BENCH_3,BENCH_4 -count 5000 -ramp 2ms

# ─── Cleanup ──────────────────────────────────────────────────

clean:
	rm -rf bin/ internal/web/dist/
	cd web && rm -rf node_modules/ .bun

# 清理测试环境
test-clean:
	rm -f config_test.yaml caster_test.db caster_test.db-wal caster_test.db-shm
