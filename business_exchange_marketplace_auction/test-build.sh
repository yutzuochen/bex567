#!/bin/bash

# 測試腳本 - 只構建Docker映像但不推送
set -e

# 配置變數
PROJECT_ID="businessexchange-468413"
SERVICE_NAME="auction-service"
IMAGE_TAG=${1:-latest}
IMAGE_NAME="gcr.io/${PROJECT_ID}/${SERVICE_NAME}"

echo "🧪 測試 Docker 構建..."
echo "項目ID: $PROJECT_ID"
echo "服務名稱: $SERVICE_NAME"
echo "鏡像: $IMAGE_NAME:$IMAGE_TAG"

# 構建 Docker 映像
echo "🏗️ 構建 Docker 映像..."
docker build -t $IMAGE_NAME:$IMAGE_TAG .

echo "✅ Docker 映像構建成功！"
echo "📋 映像標籤: $IMAGE_NAME:$IMAGE_TAG"

# 顯示映像信息
echo "📊 映像信息:"
docker images | grep $SERVICE_NAME || echo "映像列表中未找到 $SERVICE_NAME"