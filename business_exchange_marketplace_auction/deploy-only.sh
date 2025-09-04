#!/bin/bash

set -e

# 顏色設定
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置變數
PROJECT_ID="businessexchange-468413"
SERVICE_NAME="business-exchange-auction"
REGION="us-central1"
IMAGE_TAG=${1:-latest}
IMAGE_NAME="gcr.io/${PROJECT_ID}/${SERVICE_NAME}"

echo -e "${BLUE}🚢 重新部署拍賣服務到 Cloud Run...${NC}"
echo -e "項目ID: ${GREEN}$PROJECT_ID${NC}"
echo -e "服務名稱: ${GREEN}$SERVICE_NAME${NC}"
echo -e "區域: ${GREEN}$REGION${NC}"
echo -e "鏡像: ${GREEN}$IMAGE_NAME:$IMAGE_TAG${NC}"

# 替換部署配置中的 PROJECT_ID 和鏡像標籤
echo -e "${BLUE}📝 準備部署配置...${NC}"
sed -e "s/PROJECT_ID/$PROJECT_ID/g" -e "s|gcr.io/PROJECT_ID/business-exchange-auction:latest|$IMAGE_NAME:$IMAGE_TAG|g" deploy.yaml > deploy-temp.yaml

echo -e "${YELLOW}🔍 檢查生成的配置...${NC}"
echo "--- readinessProbe 配置 ---"
grep -A 10 "readinessProbe:" deploy-temp.yaml || echo "未找到 readinessProbe"

# 部署到 Cloud Run
echo -e "${BLUE}🚢 部署到 Cloud Run...${NC}"
gcloud run services replace deploy-temp.yaml --region=$REGION

# 清理臨時文件
rm deploy-temp.yaml

# 獲取服務 URL
SERVICE_URL=$(gcloud run services describe $SERVICE_NAME --region=$REGION --format="value(status.url)")

echo -e "${GREEN}✅ 部署完成！${NC}"
echo -e "${BLUE}🌐 服務 URL: ${GREEN}$SERVICE_URL${NC}"
echo -e "${BLUE}🔗 健康檢查: ${GREEN}$SERVICE_URL/healthz${NC}"
echo -e "${BLUE}📊 WebSocket 統計: ${GREEN}$SERVICE_URL/ws/stats${NC}"

# 等待服務啟動
echo -e "${BLUE}⏳ 等待服務啟動...${NC}"
sleep 15

# 檢查健康狀態
echo -e "${BLUE}🏥 檢查服務健康狀態...${NC}"
if curl -f -s "$SERVICE_URL/healthz" > /dev/null; then
    echo -e "${GREEN}✅ 服務健康且正常運行！${NC}"
    echo -e "${BLUE}🎉 部署成功完成！${NC}"
else
    echo -e "${YELLOW}⚠️  服務可能還沒完全就緒，請稍後手動檢查${NC}"
    echo -e "${YELLOW}檢查日誌: gcloud run services logs read $SERVICE_NAME --region=$REGION${NC}"
fi