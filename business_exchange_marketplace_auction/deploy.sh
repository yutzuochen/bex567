#!/bin/bash

set -e

# 配置變數
PROJECT_ID=${GOOGLE_CLOUD_PROJECT}
SERVICE_NAME="auction-service"
REGION="asia-east1"
IMAGE_TAG=${1:-latest}

echo "🚀 Starting deployment process..."
echo "Project ID: $PROJECT_ID"
echo "Service Name: $SERVICE_NAME"
echo "Region: $REGION"
echo "Image Tag: $IMAGE_TAG"

# 檢查必要的環境變數
if [ -z "$PROJECT_ID" ]; then
    echo "❌ Error: GOOGLE_CLOUD_PROJECT environment variable is not set"
    exit 1
fi

# 構建 Docker 映像
echo "🏗️  Building Docker image..."
docker build -t gcr.io/$PROJECT_ID/$SERVICE_NAME:$IMAGE_TAG .

# 推送到 Google Container Registry
echo "📦 Pushing image to GCR..."
docker push gcr.io/$PROJECT_ID/$SERVICE_NAME:$IMAGE_TAG

# 替換部署配置中的 PROJECT_ID
sed "s/PROJECT_ID/$PROJECT_ID/g" deploy.yaml > deploy-temp.yaml

# 部署到 Cloud Run
echo "🚢 Deploying to Cloud Run..."
gcloud run services replace deploy-temp.yaml --region=$REGION

# 清理臨時文件
rm deploy-temp.yaml

# 獲取服務 URL
SERVICE_URL=$(gcloud run services describe $SERVICE_NAME --region=$REGION --format="value(status.url)")

echo "✅ Deployment completed successfully!"
echo "🌐 Service URL: $SERVICE_URL"
echo "🔗 Health Check: $SERVICE_URL/healthz"

# 等待服務啟動
echo "⏳ Waiting for service to be ready..."
sleep 10

# 檢查健康狀態
if curl -f -s "$SERVICE_URL/healthz" > /dev/null; then
    echo "✅ Service is healthy!"
else
    echo "⚠️  Service might not be ready yet. Please check manually."
fi