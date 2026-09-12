FROM golang:1.25-alpine AS builder
WORKDIR /app

# Dependencies are downloaded from the committed go.mod and go.sum, not resolved
# by `go mod tidy` during the build. tidy is network-dependent and can silently
# change resolved versions between two builds of the same commit, which is the
# opposite of what an image build should do.
COPY go.mod go.sum ./
RUN go mod download

COPY . /app

ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -a -installsuffix cgo \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o hades ./cmd/hades

# The protoc plugin the SDK worker execs is built from the version in go.mod,
# not from @latest. A floating plugin generates code with a different protobuf
# runtime than the server was compiled against.
RUN go build -o /out/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go

FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache curl ca-certificates && \
    curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.70.0/buf-Linux-x86_64" \
      -o /usr/local/bin/buf && \
    chmod +x /usr/local/bin/buf && \
    apk del curl

COPY --from=builder /app/hades /app/hades
COPY --from=builder /out/protoc-gen-go /usr/local/bin/protoc-gen-go

# The default configuration must start without operator input.
#
# sample.yaml was baked in as the default, and it points at TLS certificates
# that are gitignored and never generated or copied, so a stock container failed
# in ListenAndServeTLS. container.yaml is the same file with no TLS material:
# terminate TLS at the proxy, or mount your own config over this path.
COPY --from=builder /app/config/container.yaml /app/config/config.yaml

# Writable state for the default (SQLite + gogit + disk) configuration.
RUN mkdir -p /app/_data && \
    adduser -D -u 10001 hades && \
    chown -R hades:hades /app
USER hades

EXPOSE 50051

# The registry answers gRPC over h2c, so the health check asks for something an
# HTTP client can read: an unknown procedure returns a Connect error, which is
# proof the server is up and routing.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null --post-data '{}' \
        --header 'Content-Type: application/json' \
        http://127.0.0.1:50051/healthz 2>/dev/null || exit 1

CMD ["./hades", "serve", "--config", "/app/config/config.yaml"]
