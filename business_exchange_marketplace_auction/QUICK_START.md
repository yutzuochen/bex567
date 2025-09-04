# 🚀 拍賣服務 Cloud Run 快速部署

## ⚡ 快速開始

如果你已經有 Google Cloud 項目和必要的資源，可以按照以下步驟快速部署：

### 1. 準備環境
```bash
# 設置項目ID
export GOOGLE_CLOUD_PROJECT="your-project-id"

# 確保已登入 Google Cloud
gcloud auth login
gcloud config set project $GOOGLE_CLOUD_PROJECT

# 進入項目目錄
cd business_exchange_marketplace_auction/
```

### 2. 設置 Secrets (首次部署)
```bash
# 運行自動化 secrets 設置腳本
./setup-secrets.sh
```

這個腳本會引導你設置：
- 數據庫連接信息 (Cloud SQL)
- Redis 連接信息 (Cloud Memorystore)
- JWT Secret
- 服務帳戶權限

### 3. 執行部署
```bash
# 運行部署腳本
./quick-deploy.sh
```

部署腳本會自動：
- ✅ 檢查必要的 API 是否已啟用
- ✅ 檢查 secrets 和服務帳戶
- 🏗️ 構建 Docker 鏡像
- 📦 推送到 Container Registry
- 🚢 部署到 Cloud Run
- 🏥 驗證服務健康狀態

### 4. 驗證部署
部署完成後，你會看到服務 URL，可以通過以下方式驗證：

```bash
# 檢查健康狀態
curl https://your-service-url/healthz

# 檢查 WebSocket 統計
curl https://your-service-url/ws/stats
```

## 📋 部署前檢查清單

### 必要資源
- [ ] Google Cloud 項目
- [ ] 已啟用的 API：
  - [ ] Cloud Build API
  - [ ] Cloud Run API
  - [ ] Container Registry API
- [ ] Cloud SQL MySQL 實例 (或其他 MySQL 數據庫)
- [ ] Cloud Memorystore Redis 實例 (或其他 Redis)

### 權限要求
- [ ] Cloud Build Editor
- [ ] Cloud Run Admin
- [ ] Container Registry Admin
- [ ] Secret Manager Admin
- [ ] Service Account Admin

## 🛠️ 可用的腳本

1. **`setup-secrets.sh`** - 設置必要的 secrets 和服務帳戶
2. **`quick-deploy.sh`** - 完整的自動化部署流程
3. **`deploy.sh`** - 原始的部署腳本

## 🔧 配置選項

### 資源配置
- **CPU**: 1-2 cores
- **記憶體**: 512Mi-2Gi  
- **並發**: 100 requests
- **自動擴展**: 1-10 實例

### 環境變數
服務會自動從 secrets 中讀取以下配置：
- 數據庫連接 (`auction-db-config`)
- Redis 連接 (`auction-redis-config`)
- JWT 密鑰 (`auction-jwt-config`)

## 🔍 故障排除

### 常見問題

1. **權限錯誤**
   ```bash
   # 檢查當前用戶權限
   gcloud projects get-iam-policy $GOOGLE_CLOUD_PROJECT
   ```

2. **API 未啟用**
   ```bash
   # 手動啟用必要的 API
   gcloud services enable cloudbuild.googleapis.com
   gcloud services enable run.googleapis.com
   gcloud services enable containerregistry.googleapis.com
   ```

3. **Secrets 缺失**
   ```bash
   # 檢查 secrets 是否存在
   gcloud secrets list --filter="name~auction"
   ```

4. **查看服務日誌**
   ```bash
   gcloud run services logs read auction-service --region=asia-east1
   ```

## 📖 詳細文檔

需要更詳細的設置指南？請參考：
- [`DEPLOYMENT_GUIDE.md`](./DEPLOYMENT_GUIDE.md) - 完整部署指南
- [`README.md`](./README.md) - 項目說明文檔

## 🆘 需要幫助？

如果遇到問題：
1. 檢查 Cloud Console 中的錯誤訊息
2. 查看 Cloud Run 服務日誌  
3. 確認 secrets 和權限設置正確
4. 參考詳細的部署指南