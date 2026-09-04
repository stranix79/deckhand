# Multi-stage: build with the Go toolchain, run on distroless (no shell).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN cp CHANGELOG.md docs/CHANGELOG.md && \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/stranix79/deckhand/internal/version.Version=${VERSION}" -o /out/deckhand ./cmd/deckhand

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/deckhand /deckhand
USER nonroot
VOLUME ["/data"]
ENV DECKHAND_ADDR=:8080 DECKHAND_DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/deckhand"]
CMD ["serve"]
