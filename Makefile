.PHONY: dev-frontend dev-backend build build-frontend build-backend clean \
       simbase simrover

dev-frontend:
	cd web && pnpm dev

dev-backend:
	go run ./cmd/caster

build: build-frontend build-backend

build-frontend:
	cd web && pnpm build

build-backend:
	go build -o bin/caster ./cmd/caster
	go build -o bin/simbase ./cmd/simbase
	go build -o bin/simrover ./cmd/simrover

simbase:
	go run ./cmd/simbase $(ARGS)

simrover:
	go run ./cmd/simrover $(ARGS)

clean:
	rm -rf bin/ internal/web/dist/
	cd web && rm -rf node_modules/
