BIN     := deckhand
PKG     := ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/stranix79/deckhand/internal/version.Version=$(VERSION)

.PHONY: build test lint fmt validate run-local run-hub release clean

build:
	cp CHANGELOG.md docs/CHANGELOG.md
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/deckhand

test:
	go test -race -count=1 $(PKG)

lint:
	@test -z "$$(gofmt -l . | grep -v '^site/')" || (echo "gofmt:"; gofmt -l .; exit 1)
	go vet $(PKG)
	golangci-lint run

fmt:
	gofmt -w cmd internal

validate: build
	./$(BIN) validate examples/ship-it

run-local: build
	./$(BIN) present examples/ship-it --open

run-hub:
	docker compose -f docker-compose.hub.yml up --build

# Local hub without Docker: a throwaway PostgreSQL in ./.pg (needs Homebrew postgresql).
dev-pg:
	scripts/dev-pg.sh start

dev-hub: build dev-pg
	DECKHAND_PG=postgres://localhost:5499/deckhand?sslmode=disable DECKHAND_SECRET=dev-secret-dev-secret-dev-secret-dev DECKHAND_DEV_LOG_MAGIC_LINKS=1 ./$(BIN) serve --addr :8080

release:
	goreleaser release --snapshot --clean

clean:
	rm -rf $(BIN) dist coverage.out
