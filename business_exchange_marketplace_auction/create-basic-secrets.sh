#!/bin/bash

set -e

echo "🔐 創建基本的 secrets 用於測試部署..."

# 創建數據庫配置 secret (使用測試值)
echo "創建 auction-db-config..."
echo "host=localhost:3306
user=root
password=password
database=business_exchange" | gcloud secrets create auction-db-config --data-file=- || echo "Secret 已存在，跳過"

# 創建 Redis 配置 secret (使用測試值)
echo "創建 auction-redis-config..."  
echo "host=localhost:6379" | gcloud secrets create auction-redis-config --data-file=- || echo "Secret 已存在，跳過"

# 創建 JWT 配置 secret (使用測試值)
echo "創建 auction-jwt-config..."
echo "secret=test-jwt-secret-for-deployment" | gcloud secrets create auction-jwt-config --data-file=- || echo "Secret 已存在，跳過"

echo "✅ 基本 secrets 創建完成！"

# 賦予服務帳戶權限
SA_EMAIL="auction-service-sa@businessexchange-468413.iam.gserviceaccount.com"

for secret in "auction-db-config" "auction-redis-config" "auction-jwt-config"; do
    echo "賦予 $secret 權限給服務帳戶..."
    gcloud secrets add-iam-policy-binding "$secret" \
        --member="serviceAccount:$SA_EMAIL" \
        --role="roles/secretmanager.secretAccessor" \
        --quiet || echo "權限已存在"
done

echo "🎉 Secrets 和權限設置完成！"