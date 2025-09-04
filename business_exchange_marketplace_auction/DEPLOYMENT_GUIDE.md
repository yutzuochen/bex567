# 拍賣服務 Cloud Run 部署指南

## 前置準備

### 1. 安裝和配置工具
```bash
# 安裝 Google Cloud CLI (如果尚未安裝)
# 參考: https://cloud.google.com/sdk/docs/install

# 登入 Google Cloud
gcloud auth login

# 設置項目 (替換成你的項目ID)
export GOOGLE_CLOUD_PROJECT="your-project-id"
gcloud config set project $GOOGLE_CLOUD_PROJECT

# 啟用必要的API
gcloud services enable cloudbuild.googleapis.com
gcloud services enable run.googleapis.com
gcloud services enable containerregistry.googleapis.com
gcloud services enable sqladmin.googleapis.com
gcloud services enable redis.googleapis.com
```

### 2. 設置數據庫和 Redis (如果還沒有的話)

#### Cloud SQL (MySQL)
```bash
# 創建 Cloud SQL 實例
gcloud sql instances create auction-db \
    --database-version=MYSQL_8_0 \
    --tier=db-n1-standard-1 \
    --region=asia-east1

# 創建數據庫
gcloud sql databases create business_exchange --instance=auction-db

# 創建用戶
gcloud sql users create app-user \
    --instance=auction-db \
    --password=SECURE_PASSWORD_HERE
```

#### Cloud Memorystore (Redis)
```bash
# 創建 Redis 實例
gcloud redis instances create auction-redis \
    --size=1 \
    --region=asia-east1 \
    --redis-version=redis_6_x
```

### 3. 創建 Secrets

```bash
# 數據庫配置
gcloud secrets create auction-db-config --data-file=- <<EOF
host=CLOUD_SQL_CONNECTION_NAME
user=app-user
password=SECURE_PASSWORD_HERE
database=business_exchange
EOF

# Redis 配置
gcloud secrets create auction-redis-config --data-file=- <<EOF
host=REDIS_IP_ADDRESS:6379
EOF

# JWT 配置
gcloud secrets create auction-jwt-config --data-file=- <<EOF
secret=YOUR_SUPER_SECRET_JWT_KEY_HERE
EOF
```

### 4. 創建服務賬戶
```bash
# 創建服務賬戶
gcloud iam service-accounts create auction-service-sa \
    --display-name="Auction Service Account"

# 賦予必要權限
gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT \
    --member="serviceAccount:auction-service-sa@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
    --role="roles/cloudsql.client"

gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT \
    --member="serviceAccount:auction-service-sa@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
    --role="roles/redis.editor"

# 賦予讀取 secrets 的權限
for secret in auction-db-config auction-redis-config auction-jwt-config; do
    gcloud secrets add-iam-policy-binding $secret \
        --member="serviceAccount:auction-service-sa@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
        --role="roles/secretmanager.secretAccessor"
done
```

## 部署流程

### 1. 檢查配置
```bash
cd /home/mason/Documents/bex567/business_exchange_marketplace_auction

# 檢查環境變數
echo "Project ID: $GOOGLE_CLOUD_PROJECT"

# 確認 Docker 在運行
docker --version
```

### 2. 執行部署
```bash
# 賦予執行權限
chmod +x deploy.sh

# 執行部署
./deploy.sh
```

### 3. 驗證部署
部署完成後，腳本會顯示服務 URL，你可以通過以下方式驗證：

```bash
# 檢查健康狀態
curl https://your-service-url/healthz

# 檢查 WebSocket 統計
curl https://your-service-url/ws/stats
```

## 配置說明

### 環境變數映射
- `APP_ENV=production`
- `APP_PORT=8081`
- `DB_HOST` → 從 `auction-db-config` secret 讀取
- `DB_USER` → 從 `auction-db-config` secret 讀取
- `DB_PASSWORD` → 從 `auction-db-config` secret 讀取
- `DB_NAME` → 從 `auction-db-config` secret 讀取
- `REDIS_HOST` → 從 `auction-redis-config` secret 讀取
- `JWT_SECRET` → 從 `auction-jwt-config` secret 讀取

### 資源配置
- CPU: 1-2 cores
- 記憶體: 512Mi-2Gi
- 最小實例: 1
- 最大實例: 10
- 並發: 100

### 健康檢查
- 路徑: `/healthz`
- 端口: 8081
- 初始延遲: 10-30 秒

## 故障排除

### 1. 查看日誌
```bash
gcloud run services logs read auction-service --region=asia-east1
```

### 2. 檢查配置
```bash
gcloud run services describe auction-service --region=asia-east1
```

### 3. 更新配置
如需更新環境變數或配置，修改 `deploy.yaml` 後重新執行部署腳本。

## 維護操作

### 更新服務
```bash
# 重新部署 (使用新的 image tag)
./deploy.sh v1.0.1
```

### 擴展服務
```bash
# 修改 deploy.yaml 中的 maxScale 設置
# 然後重新部署
```

### 監控
- Cloud Run 控制台: https://console.cloud.google.com/run
- Cloud Logging: https://console.cloud.google.com/logs
- Cloud Monitoring: https://console.cloud.google.com/monitoring