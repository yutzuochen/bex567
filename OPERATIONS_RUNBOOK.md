# 🚀 Business Exchange Marketplace - 運維 Runbook

## 📋 目錄

- [系統概覽](#系統概覽)
- [服務架構](#服務架構)  
- [部署流程](#部署流程)
- [日常運維](#日常運維)
- [故障處理](#故障處理)
- [壓力測試](#壓力測試)
- [災難恢復](#災難恢復)
- [緊急聯絡](#緊急聯絡)

---

## 🏗️ 系統概覽

### 專案資訊
- **專案名稱**: Business Exchange Marketplace (企業互惠平台)
- **專案 ID**: `businessexchange-468413`
- **主要語言**: Go 1.23, Next.js 14.2.5, TypeScript
- **雲端平台**: Google Cloud Platform
- **部署方式**: Google Cloud Run (無伺服器容器)

### 服務清單
| 服務名稱 | 功能 | 端口 | 技術棧 | 狀態頁面 |
|---------|------|------|--------|----------|
| **主平台後端** | 用戶管理、商業清單、交易 | 8080 | Go + Gin + MySQL | `/healthz` |
| **拍賣服務** | 英式拍賣、封閉式拍賣、WebSocket | 8081 | Go + Gin + WebSocket | `/healthz` |
| **前端應用** | React 用戶界面 | 3000 | Next.js + TypeScript | `/api/healthz` |
| **MySQL** | 主資料庫 | 3306 | MySQL 8.0 | - |
| **Redis** | 快取 + Session + WebSocket | 6379 | Redis 7 | - |

---

## 🏛️ 服務架構

### 部署架構
```
Internet → Cloud Load Balancer → Cloud Run Services
                                ├── Frontend Service (Next.js)
                                ├── Backend Service (Main API)  
                                └── Auction Service (WebSocket)
                                        ↓
                          Cloud SQL (MySQL) + Memorystore (Redis)
```

### 數據流向
1. **用戶認證流**: Frontend → Backend → JWT → Redis Session
2. **拍賣流程**: Frontend → Auction Service → WebSocket Hub → Redis Pub/Sub
3. **資料存取**: Services → Cloud SQL → Redis Cache

---

## 🚀 部署流程

### 自動部署 (推薦)

#### 前端部署
```bash
cd business_exchange_marketplace_frontend/
./deploy-frontend.sh
```

#### 拍賣服務部署  
```bash
cd business_exchange_marketplace_auction/
export GOOGLE_CLOUD_PROJECT="businessexchange-468413"
./deploy.sh
```

### 手動部署步驟

#### 1. 前置作業
```bash
# 認證 Google Cloud
gcloud auth login
gcloud config set project businessexchange-468413

# 啟用必要 API
gcloud services enable cloudbuild.googleapis.com
gcloud services enable run.googleapis.com
gcloud services enable containerregistry.googleapis.com
```

#### 2. 構建 & 部署前端
```bash
# 構建映像
docker build -f Dockerfile.production \
    --build-arg NEXT_PUBLIC_API_URL=https://your-backend-url \
    -t gcr.io/businessexchange-468413/business-exchange-frontend .

# 推送映像
docker push gcr.io/businessexchange-468413/business-exchange-frontend

# 部署到 Cloud Run
gcloud run deploy business-exchange-frontend \
    --image gcr.io/businessexchange-468413/business-exchange-frontend \
    --platform managed \
    --region us-central1 \
    --allow-unauthenticated \
    --memory 1Gi \
    --cpu 1 \
    --max-instances 10
```

#### 3. 構建 & 部署拍賣服務
```bash
# 構建映像
docker build -t gcr.io/businessexchange-468413/auction-service .

# 推送映像  
docker push gcr.io/businessexchange-468413/auction-service

# 部署到 Cloud Run
gcloud run services replace deploy.yaml --region=asia-east1
```

### 部署檢查清單
- [ ] 環境變數正確設置
- [ ] 資料庫遷移完成
- [ ] 健康檢查通過
- [ ] Load Balancer 配置更新
- [ ] DNS 記錄更新
- [ ] SSL 證書有效
- [ ] 監控警報正常

---

## 🔧 日常運維

### 服務健康檢查

#### 自動化健康檢查
```bash
# 檢查所有服務狀態
./scripts/health-check-all.sh

# 檢查特定服務
curl -f https://your-frontend-url/api/healthz
curl -f https://your-auction-url/healthz
curl -f https://your-backend-url/healthz
```

#### 服務狀態查詢
```bash
# Cloud Run 服務狀態
gcloud run services list --region=us-central1

# 詳細服務資訊
gcloud run services describe business-exchange-frontend \
    --region=us-central1 \
    --format="table(metadata.name,status.url,status.conditions[0].status)"

# Cloud SQL 狀態
gcloud sql instances list

# Redis (Memorystore) 狀態  
gcloud redis instances list --region=us-central1
```

### 日誌管理

#### 查看服務日誌
```bash
# 前端服務日誌
gcloud logs read --service=business-exchange-frontend --limit=100

# 拍賣服務日誌
gcloud logs read --service=auction-service --limit=100

# 即時日誌監控
gcloud logs tail --service=auction-service

# 錯誤日誌過濾
gcloud logs read --service=auction-service \
    --filter='severity>=ERROR' --limit=50
```

#### 結構化日誌查詢
```bash
# WebSocket 連接錯誤
gcloud logs read --filter='
    resource.type="cloud_run_revision"
    AND jsonPayload.message:"WebSocket connection failed"
    AND severity>=ERROR
' --limit=20

# 資料庫連接問題
gcloud logs read --filter='
    jsonPayload.message:"database connection"
    AND severity>=WARNING
' --limit=20
```

### 資料庫維護

#### 備份策略
```bash
# 創建資料庫備份
gcloud sql backups create --instance=business-exchange-db

# 查看備份清單
gcloud sql backups list --instance=business-exchange-db

# 定期備份檢查 (每日)
gcloud sql backups list --instance=business-exchange-db \
    --filter="startTime.date('%Y-%m-%d')='$(date +%Y-%m-%d)'"
```

#### 資料庫維護
```bash
# 執行資料庫遷移
cd business_exchange_marketplace/
make migrate

# 檢查遷移狀態
make migrate-status

# 強制重置遷移版本 (謹慎使用)
go run ./cmd/migrate -action=force -version=17
```

### 效能監控

#### 重要指標查詢
```bash
# CPU 使用率
gcloud monitoring metrics list --filter="metric.type:cpu"

# 記憶體使用率
gcloud monitoring metrics list --filter="metric.type:memory"

# 請求延遲
gcloud monitoring metrics list --filter="metric.type:request_latency"

# WebSocket 連接數
curl -s https://your-auction-url/ws/stats | jq '.total_connections'
```

---

## 🚨 故障處理

### 故障診斷流程圖
```
故障報告 → 影響評估 → 立即響應 → 根因分析 → 修復 → 事後檢討
```

### 常見故障場景

#### 🔴 **高優先級故障**

##### 場景 1: 服務完全無法訪問
**症狀**: HTTP 500/502/503 錯誤，服務不響應

**立即響應**:
1. 檢查 Cloud Run 服務狀態
   ```bash
   gcloud run services list --region=us-central1
   ```

2. 查看最近部署
   ```bash
   gcloud run revisions list --service=business-exchange-frontend
   ```

3. 回滾到上一版本
   ```bash
   gcloud run services update-traffic business-exchange-frontend \
       --to-revisions=PREVIOUS_REVISION=100
   ```

4. 檢查資料庫連接
   ```bash
   gcloud sql instances describe business-exchange-db
   ```

**根因分析**:
- 檢查部署日誌
- 分析應用程式日誌
- 檢查資源限制
- 驗證環境變數

##### 場景 2: 資料庫連接失敗
**症狀**: 資料庫相關錯誤，查詢超時

**立即響應**:
1. 檢查 Cloud SQL 狀態
   ```bash
   gcloud sql instances list
   gcloud sql operations list --instance=business-exchange-db
   ```

2. 檢查連接池
   ```bash
   # 查看活躍連接數
   gcloud sql instances describe business-exchange-db \
       --format="value(databaseFlags[].value)"
   ```

3. 重啟服務 (如果必要)
   ```bash
   gcloud run deploy business-exchange-frontend \
       --image gcr.io/businessexchange-468413/business-exchange-frontend
   ```

##### 場景 3: WebSocket 連接異常
**症狀**: 拍賣實時更新失效，連接頻繁斷開

**立即響應**:
1. 檢查 WebSocket 統計
   ```bash
   curl -s https://your-auction-url/ws/stats
   ```

2. 檢查 Redis 連接
   ```bash
   gcloud redis instances list --region=us-central1
   ```

3. 重啟拍賣服務
   ```bash
   gcloud run deploy auction-service \
       --image gcr.io/businessexchange-468413/auction-service
   ```

#### 🟡 **中優先級故障**

##### 場景 4: 效能降級
**症狀**: 響應時間增加，但服務可用

**診斷步驟**:
1. 檢查資源使用率
   ```bash
   gcloud monitoring metrics list --filter="metric.type:cpu"
   ```

2. 分析慢查詢
   ```bash
   gcloud sql instances describe business-exchange-db \
       --format="value(settings.insightsConfig)"
   ```

3. 檢查快取命中率
   ```bash
   gcloud redis instances describe redis-instance \
       --region=us-central1
   ```

**解決方案**:
- 增加 Cloud Run 實例數量
- 優化資料庫查詢
- 調整快取策略
- 增加資源配額

### 故障響應時間目標 (SLA)

| 故障等級 | 響應時間 | 解決時間 |
|---------|----------|----------|
| **P0 - 嚴重** | 15 分鐘 | 4 小時 |
| **P1 - 高** | 1 小時 | 24 小時 |
| **P2 - 中** | 4 小時 | 72 小時 |
| **P3 - 低** | 24 小時 | 1 週 |

### 故障通知機制
- **P0/P1**: 立即電話 + SMS + Email
- **P2**: Email + Slack
- **P3**: 工單系統

---

## 🧪 壓力測試

### 測試場景規劃

#### 場景 1: 基線負載測試
```bash
# 使用 Apache Bench 測試基本 API
ab -n 1000 -c 10 https://your-backend-url/api/v1/listings

# 使用 wrk 測試高並發
wrk -t12 -c400 -d30s https://your-frontend-url/
```

#### 場景 2: WebSocket 壓力測試
```bash
# 創建 WebSocket 壓力測試腳本
cat > ws-stress-test.js << 'EOF'
const WebSocket = require('ws');

const concurrent = 100;
const duration = 60000; // 60 seconds
let connections = [];
let messages = 0;

for(let i = 0; i < concurrent; i++) {
    const ws = new WebSocket('wss://your-auction-url/ws/auctions/1?token=TEST_TOKEN');
    
    ws.on('open', function open() {
        console.log(`Connection ${i} established`);
        
        // Send periodic messages
        const interval = setInterval(() => {
            ws.send(JSON.stringify({type: 'heartbeat', data: {}}));
            messages++;
        }, 1000);
        
        setTimeout(() => {
            clearInterval(interval);
            ws.close();
        }, duration);
    });
    
    ws.on('message', function message(data) {
        console.log(`Received: ${data}`);
    });
    
    connections.push(ws);
}

setTimeout(() => {
    console.log(`Test completed. Messages sent: ${messages}`);
    process.exit(0);
}, duration + 5000);
EOF

node ws-stress-test.js
```

#### 場景 3: 資料庫壓力測試
```bash
# 使用 sysbench 測試資料庫效能
sysbench oltp_read_write \
    --mysql-host=your-cloud-sql-ip \
    --mysql-port=3306 \
    --mysql-user=app \
    --mysql-password=your-password \
    --mysql-db=business_exchange \
    --threads=16 \
    --time=300 \
    run
```

### 預期效能指標

#### API 效能基準
| 端點類型 | 目標延遲 (P95) | 目標 TPS | 錯誤率 |
|---------|----------------|----------|--------|
| **健康檢查** | < 50ms | 1000+ | < 0.1% |
| **用戶認證** | < 200ms | 500+ | < 1% |
| **商業清單** | < 300ms | 200+ | < 2% |
| **拍賣 API** | < 500ms | 100+ | < 2% |
| **WebSocket** | < 100ms | 500+ 連接 | < 5% |

#### 資源使用限制
| 資源類型 | 告警閾值 | 緊急閾值 |
|---------|----------|----------|
| **CPU** | 70% | 85% |
| **記憶體** | 80% | 90% |
| **資料庫連接** | 80% | 95% |
| **磁碟空間** | 85% | 95% |

### 壓力測試執行計畫

#### 預生產測試 (每週)
1. **負載測試**: 模擬正常流量 2x
2. **峰值測試**: 模擬預期最大負載 5x  
3. **耐久測試**: 持續運行 24 小時
4. **故障恢復**: 模擬服務中斷後恢復

#### 生產前測試 (發布前)
1. **煙霧測試**: 基本功能驗證
2. **回歸測試**: 自動化測試套件
3. **金絲雀測試**: 小流量真實用戶測試
4. **容量測試**: 確認資源配置充足

---

## 🚑 災難恢復

### 備份策略

#### 資料庫備份
```bash
# 自動每日備份設置
gcloud sql instances patch business-exchange-db \
    --backup-start-time=03:00 \
    --backup-location=us \
    --enable-bin-log

# 手動備份
gcloud sql backups create \
    --instance=business-exchange-db \
    --description="Pre-release backup $(date +%Y%m%d)"
```

#### 配置備份
```bash
# 導出 Cloud Run 配置
gcloud run services describe business-exchange-frontend \
    --region=us-central1 \
    --format=export > frontend-config-backup.yaml

# 導出 IAM 政策
gcloud projects get-iam-policy businessexchange-468413 \
    --format=json > iam-policy-backup.json
```

### 災難恢復程序

#### RTO/RPO 目標
| 服務等級 | RTO (恢復時間) | RPO (數據丟失) |
|---------|---------------|---------------|
| **前端服務** | 15 分鐘 | 0 (無狀態) |
| **後端服務** | 30 分鐘 | 0 (無狀態) |
| **資料庫** | 60 分鐘 | < 1 小時 |
| **用戶會話** | 5 分鐘 | < 5 分鐘 |

#### 災難恢復測試 (每季度)
1. **資料庫恢復測試**
   ```bash
   # 創建測試實例
   gcloud sql instances clone business-exchange-db \
       disaster-recovery-test
   
   # 驗證數據完整性
   gcloud sql connect disaster-recovery-test
   ```

2. **服務恢復測試**
   ```bash
   # 部署到災難恢復環境
   gcloud run deploy dr-frontend \
       --image gcr.io/businessexchange-468413/business-exchange-frontend \
       --region=us-west1
   ```

3. **完整災難恢復演練**
   - 模擬區域性故障
   - 執行故障轉移
   - 驗證用戶訪問
   - 測試數據一致性
   - 執行故障回切

### 緊急聯絡方式

#### 升級流程
```
一線值班 → 技術主管 → 產品負責人 → 高級管理層
  (15min)     (30min)     (60min)      (4hours)
```

#### 聯絡清單
| 角色 | 主要責任 | 電話 | Email | 備用聯絡方式 |
|------|---------|------|-------|-------------|
| **值班工程師** | 即時響應、初步診斷 | [電話] | [Email] | Slack @oncall |
| **技術主管** | 技術決策、資源協調 | [電話] | [Email] | Slack @tech-lead |
| **DevOps 工程師** | 基礎設施、部署 | [電話] | [Email] | Slack @devops |
| **資料庫專家** | 資料庫問題處理 | [電話] | [Email] | Slack @dba |

#### 通知渠道
- **即時通訊**: Slack #alerts, #incidents
- **監控警報**: PagerDuty, Google Cloud Monitoring
- **狀態頁面**: https://status.yourcompany.com
- **用戶通知**: Email, In-app notifications

---

## 📚 附錄

### 常用命令速查

#### Cloud Run
```bash
# 查看所有服務
gcloud run services list

# 部署新版本  
gcloud run deploy SERVICE_NAME --image IMAGE_URL

# 設置流量分配
gcloud run services update-traffic SERVICE_NAME \
    --to-revisions=NEW_REVISION=100

# 查看服務日誌
gcloud logs read --service=SERVICE_NAME --limit=50
```

#### Cloud SQL
```bash
# 連接資料庫
gcloud sql connect INSTANCE_NAME --user=USERNAME

# 創建備份
gcloud sql backups create --instance=INSTANCE_NAME

# 查看資料庫操作
gcloud sql operations list --instance=INSTANCE_NAME
```

#### 監控相關
```bash
# 創建警報策略
gcloud alpha monitoring policies create --policy-from-file=policy.json

# 查看指標
gcloud monitoring metrics list

# 查看警報歷史
gcloud alpha monitoring notification-channels list
```

### 配置檔案模板

#### Cloud Run 服務配置
```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: business-exchange-frontend
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/maxScale: "10"
        run.googleapis.com/cpu-throttling: "false"
    spec:
      containerConcurrency: 80
      timeoutSeconds: 300
      containers:
      - image: gcr.io/PROJECT_ID/business-exchange-frontend
        resources:
          limits:
            cpu: "1"
            memory: "1Gi"
        env:
        - name: NODE_ENV
          value: "production"
```

#### 監控警報配置
```yaml
displayName: "High Error Rate"
conditions:
  - displayName: "Error rate too high"
    conditionThreshold:
      filter: 'resource.type="cloud_run_revision"'
      comparison: COMPARISON_GREATER_THAN
      thresholdValue: 0.05
      duration: "300s"
```

---

**文檔版本**: 1.0  
**最後更新**: 2024-09-03  
**維護人員**: DevOps Team  
**審核狀態**: ✅ 已審核