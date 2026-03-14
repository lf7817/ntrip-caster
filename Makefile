.PHONY: dev-frontend dev-backend build build-frontend build-backend clean

dev-frontend:
	cd web && pnpm dev

dev-backend:
	go run ./cmd/caster

build: build-frontend build-backend

build-frontend:
	cd web && pnpm build

build-backend:
	go build -o bin/caster ./cmd/caster

clean:
	rm -rf bin/ internal/web/dist/
	cd web && rm -rf node_modules/
