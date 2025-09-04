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

echo -e "${BLUE}🚀 拍賣服務 Cloud Run 部署開始...${NC}"
echo -e "項目ID: ${GREEN}$PROJECT_ID${NC}"
echo -e "服務名稱: ${GREEN}$SERVICE_NAME${NC}"
echo -e "區域: ${GREEN}$REGION${NC}"
echo -e "鏡像: ${GREEN}$IMAGE_NAME:$IMAGE_TAG${NC}"

# 檢查必要的環境變數
if [ -z "$PROJECT_ID" ]; then
    echo -e "${RED}❌ 錯誤: GOOGLE_CLOUD_PROJECT 環境變數未設置${NC}"
    echo -e "${YELLOW}請執行: export GOOGLE_CLOUD_PROJECT=your-project-id${NC}"
    exit 1
fi

# 檢查 gcloud 是否已登入
if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q "@"; then
    echo -e "${RED}❌ 錯誤: 未登入 Google Cloud${NC}"
    echo -e "${YELLOW}請執行: gcloud auth login${NC}"
    exit 1
fi

# 檢查 Docker 是否運行
if ! docker info >/dev/null 2>&1; then
    echo -e "${RED}❌ 錯誤: Docker 未運行${NC}"
    exit 1
fi

# 確認必要的服務已啟用
echo -e "${BLUE}📋 檢查必要的 API...${NC}"
REQUIRED_APIS=(
    "cloudbuild.googleapis.com"
    "run.googleapis.com"
    "containerregistry.googleapis.com"
)

for api in "${REQUIRED_APIS[@]}"; do
    if gcloud services list --enabled --filter="name:$api" --format="value(name)" | grep -q "$api"; then
        echo -e "✅ $api 已啟用"
    else
        echo -e "${YELLOW}⚠️  正在啟用 $api...${NC}"
        gcloud services enable "$api"
    fi
done

# 構建 Docker 映像
echo -e "${BLUE}🏗️  構建 Docker 映像...${NC}"
docker build -t $IMAGE_NAME:$IMAGE_TAG .

# 推送到 Google Container Registry
echo -e "${BLUE}📦 推送映像到 GCR...${NC}"
docker push $IMAGE_NAME:$IMAGE_TAG

# # 檢查必要的 secrets 是否存在
# echo -e "${BLUE}🔐 檢查 secrets...${NC}"
# REQUIRED_SECRETS=(
#     "auction-db-config"
#     "auction-redis-config"
#     "auction-jwt-config"
# )

# for secret in "${REQUIRED_SECRETS[@]}"; do
#     if gcloud secrets describe "$secret" >/dev/null 2>&1; then
#         echo -e "✅ Secret $secret 存在"
#     else
#         echo -e "${RED}❌ 錯誤: Secret $secret 不存在${NC}"
#         echo -e "${YELLOW}請參考 DEPLOYMENT_GUIDE.md 創建必要的 secrets${NC}"
#         exit 1
#     fi
# done

# 檢查服務帳戶是否存在
SA_EMAIL="auction-service-sa@$PROJECT_ID.iam.gserviceaccount.com"
if gcloud iam service-accounts describe "$SA_EMAIL" >/dev/null 2>&1; then
    echo -e "✅ 服務帳戶存在"
else
    echo -e "${YELLOW}⚠️  服務帳戶不存在，將創建...${NC}"
    gcloud iam service-accounts create auction-service-sa \
        --display-name="Auction Service Account"
    
    # 賦予必要權限
    gcloud projects add-iam-policy-binding $PROJECT_ID \
        --member="serviceAccount:$SA_EMAIL" \
        --role="roles/cloudsql.client"
    
    for secret in "${REQUIRED_SECRETS[@]}"; do
        gcloud secrets add-iam-policy-binding $secret \
            --member="serviceAccount:$SA_EMAIL" \
            --role="roles/secretmanager.secretAccessor"
    done
fi

# 替換部署配置中的 PROJECT_ID 和鏡像標籤
echo -e "${BLUE}📝 準備部署配置...${NC}"
sed -e "s/PROJECT_ID/$PROJECT_ID/g" -e "s|gcr.io/PROJECT_ID/business-exchange-auction:latest|$IMAGE_NAME:$IMAGE_TAG|g" deploy.yaml > deploy-temp.yaml

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