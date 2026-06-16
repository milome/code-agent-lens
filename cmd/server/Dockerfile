# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

# Copy module definition first for caching
COPY go.mod ./

# 启用 Go Modules
# go env -w GO111MODULE=on
# 配置国内七牛云代理（最稳定）
# go env -w GOPROXY=https://goproxy.cn,direct

# Download dependencies (go.sum will be generated after tidy)
RUN go env -w GO111MODULE=on && go env -w GOPROXY=https://goproxy.cn,direct && go mod download

# Copy source code
COPY . ./

# Ensure module graph is complete (generates go.sum)
RUN go mod tidy

# Build the headless API server (webui is embedded via go:embed)
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o code-agent-lens-server ./cmd/server

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs tzdata wget

WORKDIR /app

# Prepare data directory
RUN mkdir -p /data

# Copy binary from builder
COPY --from=builder /app/code-agent-lens-server /app/code-agent-lens-server

# Environment variables
ENV CODE_AGENT_LENS_DATA_DIR=/data
ENV CODE_AGENT_LENS_PORT=3000
ENV CODE_AGENT_LENS_DB_PATH=/data/code-agent-lens.db
ENV CODE_AGENT_LENS_BASIC_AUTH_ENABLED=true
ENV CODE_AGENT_LENS_BASIC_AUTH_USERNAME=admin

# Expose HTTP API and Debug Portal ports used by standalone and full Docker modes
EXPOSE 3000 3010 3011

# Health check targets the API health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider "http://localhost:${CODE_AGENT_LENS_PORT:-3000}/health" || exit 1

# Volume for persistent data
VOLUME ["/data"]

# Run the headless server
ENTRYPOINT ["/app/code-agent-lens-server"]
