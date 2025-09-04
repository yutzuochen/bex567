#!/bin/bash

set -e

# 顏色設定
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🔐 設置拍賣服務 Secrets${NC}"

# 檢查項目ID
PROJECT_ID=${GOOGLE_CLOUD_PROJECT}
if [ -z "$PROJECT_ID" ]; then
    echo -e "${RED}❌ 錯誤: GOOGLE_CLOUD_PROJECT 環境變數未設置${NC}"
    echo -e "${YELLOW}請執行: export GOOGLE_CLOUD_PROJECT=your-project-id${NC}"
    exit 1
fi

echo -e "項目ID: ${GREEN}$PROJECT_ID${NC}"

# 函數：安全地讀取輸入
read_secret() {
    local prompt="$1"
    local var_name="$2"
    echo -e "${YELLOW}$prompt${NC}"
    read -s value
    echo
    if [ -z "$value" ]; then
        echo -e "${RED}❌ 值不能為空${NC}"
        exit 1
    fi
    eval "$var_name='$value'"
}

# 函數：創建 secret
create_secret() {
    local secret_name="$1"
    local secret_data="$2"
    
    if gcloud secrets describe "$secret_name" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  Secret $secret_name 已存在，是否要更新？ (y/N)${NC}"
        read -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "$secret_data" | gcloud secrets versions add "$secret_name" --data-file=-
            echo -e "✅ Secret $secret_name 已更新"
        else
            echo -e "⏭️  跳過 $secret_name"
        fi
    else
        echo "$secret_data" | gcloud secrets create "$secret_name" --data-file=-
        echo -e "✅ Secret $secret_name 已創建"
    fi
}

echo -e "${BLUE}請輸入數據庫配置信息：${NC}"

# Cloud SQL 連接名稱 (格式：PROJECT_ID:REGION:INSTANCE_NAME)
read -p "Cloud SQL 連接名稱 (例如：$PROJECT_ID:asia-east1:auction-db): " db_host
if [ -z "$db_host" ]; then
    db_host="$PROJECT_ID:asia-east1:auction-db"
fi

read -p "數據庫用戶名 (默認: app-user): " db_user
if [ -z "$db_user" ]; then
    db_user="app-user"
fi

read_secret "請輸入數據庫密碼：" db_password

read -p "數據庫名稱 (默認: business_exchange): " db_name
if [ -z "$db_name" ]; then
    db_name="business_exchange"
fi

# 創建數據庫配置 secret
db_config="host=$db_host
user=$db_user
password=$db_password
database=$db_name"

create_secret "auction-db-config" "$db_config"

echo -e "${BLUE}請輸入 Redis 配置信息：${NC}"

read -p "Redis IP 地址 (例如：10.0.0.3): " redis_ip
if [ -z "$redis_ip" ]; then
    echo -e "${RED}❌ Redis IP 地址不能為空${NC}"
    exit 1
fi

read -p "Redis 端口 (默認: 6379): " redis_port
if [ -z "$redis_port" ]; then
    redis_port="6379"
fi

read -p "Redis 密碼 (如果沒有密碼請按回車): " redis_password

# 創建 Redis 配置 secret
if [ -z "$redis_password" ]; then
    redis_config="host=$redis_ip:$redis_port"
else
    redis_config="host=$redis_ip:$redis_port
password=$redis_password"
fi

create_secret "auction-redis-config" "$redis_config"

echo -e "${BLUE}請輸入 JWT 配置信息：${NC}"

read_secret "請輸入 JWT Secret (建議使用強密碼)：" jwt_secret

# 創建 JWT 配置 secret
jwt_config="secret=$jwt_secret"

create_secret "auction-jwt-config" "$jwt_config"

echo -e "${GREEN}✅ 所有 secrets 設置完成！${NC}"

# 創建服務帳戶 (如果不存在)
SA_NAME="auction-service-sa"
SA_EMAIL="$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com"

if gcloud iam service-accounts describe "$SA_EMAIL" >/dev/null 2>&1; then
    echo -e "✅ 服務帳戶已存在"
else
    echo -e "${BLUE}👤 創建服務帳戶...${NC}"
    gcloud iam service-accounts create "$SA_NAME" \
        --display-name="Auction Service Account"
    echo -e "✅ 服務帳戶已創建"
fi

# 賦予服務帳戶讀取 secrets 的權限
echo -e "${BLUE}🔑 設置權限...${NC}"
for secret in "auction-db-config" "auction-redis-config" "auction-jwt-config"; do
    gcloud secrets add-iam-policy-binding "$secret" \
        --member="serviceAccount:$SA_EMAIL" \
        --role="roles/secretmanager.secretAccessor" \
        --quiet
    echo -e "✅ 賦予 $secret 讀取權限"
done

# 賦予 Cloud SQL 客戶端權限
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="serviceAccount:$SA_EMAIL" \
    --role="roles/cloudsql.client" \
    --quiet
echo -e "✅ 賦予 Cloud SQL 客戶端權限"

echo -e "${GREEN}🎉 Secrets 和權限設置完成！${NC}"
echo -e "${YELLOW}現在可以執行部署腳本了：${NC}"
echo -e "${BLUE}./quick-deploy.sh${NC}"