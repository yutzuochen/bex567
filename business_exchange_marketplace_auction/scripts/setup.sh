#!/bin/bash

set -e

echo "🚀 Setting up Auction Service..."

# 創建必要目錄
mkdir -p tmp bin logs

# 檢查 Go 版本
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.23 or later."
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✅ Go version: $GO_VERSION"

# 複製環境變數檔案
if [ ! -f .env ]; then
    echo "📋 Creating .env file from template..."
    cp env.example .env
    echo "⚠️  Please edit .env file to set your database and Redis configuration."
else
    echo "✅ .env file already exists"
fi

# 下載依賴
echo "📦 Downloading Go dependencies..."
go mod tidy

# 檢查資料庫連線
echo "🔍 Checking database connection..."
if docker ps | grep -q mysql; then
    echo "✅ MySQL container is running"
else
    echo "⚠️  MySQL container is not running. Starting with docker-compose..."
    docker-compose -f docker-compose.dev.yml up -d mysql redis
    echo "⏳ Waiting for MySQL to be ready..."
    sleep 10
fi

# 執行資料庫遷移
echo "🗃️  Running database migrations..."
go run ./cmd/migrate -action=up

echo "🎉 Setup completed successfully!"
echo ""
echo "📚 Available commands:"
echo "  make run          - Start the server"
echo "  make build        - Build binaries"
echo "  make migrate      - Run migrations"
echo "  make finalize-job - Run finalize job"
echo ""
echo "🐳 Docker commands:"
echo "  docker-compose -f docker-compose.dev.yml up    - Start development environment"
echo "  docker-compose -f docker-compose.dev.yml down  - Stop development environment"
echo ""
echo "🌐 Once started, the API will be available at:"
echo "  http://localhost:8081/healthz   - Health check"
echo "  http://localhost:8081/api/v1/   - API endpoints"