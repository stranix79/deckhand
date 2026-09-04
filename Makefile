BIN     := deckhand
PKG     := ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/stranix79/deckhand/internal/version.Version=$(VERSION)

.PHONY: build test lint fmt validate run-local run-hub release clean

build:
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

release:
	goreleaser release --snapshot --clean

clean:
	rm -rf $(BIN) dist coverage.out
