FROM golang:1.23-alpine
WORKDIR /app
COPY . /app
RUN go mod tidy
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags '-s -w' -o hades ./cmd/hades

FROM alpine:3.21
WORKDIR /app
COPY --from=0 /app/hades /app/hades
COPY --from=0 /app/config/sample.yaml /app/config/config.yaml
COPY --from=0 /app/config/rbac_model.conf /app/config/rbac_model.conf
CMD ["./hades", "serve", "--config", "/app/config/config.yaml"]
