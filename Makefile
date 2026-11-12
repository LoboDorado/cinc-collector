VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)

DEV_DB_DSN ?= postgres://postgres:postgres@localhost:7890/postgres?sslmode=disable

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*## "}{printf "%-20s %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile the cinc-collector binary into ./bin
	go build -ldflags "$(LDFLAGS)" -o bin/cinc-collector ./cmd/cinc-collector

.PHONY: test
test: ## Run the test suite with race detection and coverage
	go test ./... -race -cover

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go source
	gofmt -w .

.PHONY: tidy
tidy: ## Tidy and vendor Go module dependencies
	go mod tidy
	go mod vendor

.PHONY: dev-up
dev-up: ## Start just the local SereneDB (run the collector natively with `make run`)
	docker compose -f dev/docker-compose.yml up -d serenedb

.PHONY: dev-down
dev-down: ## Stop the local dev stack, including the containerized collector if running
	docker compose -f dev/docker-compose.yml down

.PHONY: dev-migrate
dev-migrate: ## Apply migrations against the local dev stack
	go run ./cmd/cinc-collector migrate -db-dsn "$(DEV_DB_DSN)"

.PHONY: run
run: ## Run the server natively against the local dev stack
	CINC_COLLECTOR_DB_DSN="$(DEV_DB_DSN)" go run ./cmd/cinc-collector serve

.PHONY: dev-collector-up
dev-collector-up: ## Build and run SereneDB + migrate + collector fully in containers (run `make tidy` first if vendor/ is missing or stale)
	docker compose -f dev/docker-compose.yml up -d --build collector

.PHONY: dev-collector-logs
dev-collector-logs: ## Tail the containerized collector's logs
	docker compose -f dev/docker-compose.yml logs -f collector

DIST_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: dist
dist: ## Cross-compile release archives for $(DIST_TARGETS) into ./dist
	rm -rf dist && mkdir -p dist
	@for target in $(DIST_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		out=dist/cinc-collector-$$os-$$arch; \
		mkdir -p $$out; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$out/cinc-collector ./cmd/cinc-collector; \
		tar -C dist -czf dist/cinc-collector-$$os-$$arch.tar.gz cinc-collector-$$os-$$arch; \
		rm -rf $$out; \
	done
	@cd dist && shasum -a 256 *.tar.gz > SHA256SUMS

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin dist
