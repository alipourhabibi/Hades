FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . /app
RUN go mod tidy
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags '-s -w' -o hades ./cmd/hades
# Build protoc plugins used by the SDK generation worker.
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache curl && \
    curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.70.0/buf-Linux-x86_64" \
      -o /usr/local/bin/buf && \
    chmod +x /usr/local/bin/buf && \
    apk del curl

COPY --from=builder /app/hades /app/hades
COPY --from=builder /go/bin/protoc-gen-go /usr/local/bin/protoc-gen-go
COPY --from=builder /app/config/sample.yaml /app/config/config.yaml
COPY --from=builder /app/config/rbac_model.conf /app/config/rbac_model.conf
CMD ["./hades", "serve", "--config", "/app/config/config.yaml"]
