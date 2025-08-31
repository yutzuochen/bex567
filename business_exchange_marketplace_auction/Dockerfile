# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 安裝必要的套件
RUN apk add --no-cache git ca-certificates tzdata

# 複製 go mod 檔案
COPY go.mod go.sum ./

# 下載依賴
RUN go mod download

# 複製原始碼
COPY . .

# 編譯應用程式
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/finalize-job ./cmd/finalize-job

# Production stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# 複製編譯好的二進位檔案
COPY --from=builder /app/bin/server .
COPY --from=builder /app/bin/migrate .
COPY --from=builder /app/bin/finalize-job .

# 複製 migration 檔案
COPY --from=builder /app/migrations ./migrations

# 設定時區
ENV TZ=Asia/Taipei

# 暴露端口
EXPOSE 8081

# 健康檢查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/healthz || exit 1

# 執行服務器
CMD ["./server"]