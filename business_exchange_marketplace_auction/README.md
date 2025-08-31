# 拍賣服務 (Auction Service)

企業交易平台的拍賣服務，實施密封投標拍賣系統，支援軟關閉機制、黑名單管理、即時通知等功能。

## 🚀 功能特色

### Phase 1 - 核心拍賣功能 (已實施)
- ✅ 密封投標拍賣系統
- ✅ 軟關閉機制 (anti-sniping)
- ✅ 價格區間驗證
- ✅ 黑名單管理
- ✅ 匿名化出價者顯示
- ✅ 拍賣結束自動排名
- ✅ 通知系統 (前7名 + 其他參與者)
- ✅ 審計日誌追蹤

### 拍賣規則
- **拍賣類型**：密封投標（盲標）
- **拍賣期間**：1-61天
- **價格機制**：業主設定價格區間，投標者只能在區間內出價
- **軟關閉**：結束前3分鐘若有人出價，自動延長1分鐘
- **匿名化**：顯示為 "Bidder #23" 等匿名標籤
- **黑名單**：工作人員手動管理，阻止惡意用戶參與

## 🏗️ 技術架構

- **語言**: Go 1.23
- **框架**: Gin Web Framework
- **資料庫**: MySQL 8.0 + GORM
- **快取**: Redis (預留)
- **認證**: JWT
- **日誌**: Zap 結構化日誌
- **部署**: Docker + Google Cloud Run

## 📦 安裝與執行

### 本地開發

```bash
# 1. 複製環境變數
cp env.example .env
# 編輯 .env 設定資料庫連線等資訊

# 2. 啟動資料庫 (使用 Docker Compose)
docker-compose -f docker-compose.dev.yml up -d mysql redis

# 3. 執行 migrations
make migrate

# 4. 啟動服務
make run
```

### Docker 開發環境

```bash
# 啟動完整開發環境 (包含熱重載)
docker-compose -f docker-compose.dev.yml up --build

# 查看日誌
docker-compose -f docker-compose.dev.yml logs -f auction-service

# 停止服務
docker-compose -f docker-compose.dev.yml down
```

### 生產環境

```bash
# 啟動生產環境
docker-compose up --build -d

# 執行 finalize-job (定時任務)
docker-compose run --rm finalize-job
```

## 📚 API 文件

### 認證
所有需要認證的 API 都需要在 Header 中包含：
```
Authorization: Bearer <JWT_TOKEN>
```

### 拍賣管理
- `POST /api/v1/auctions` - 創建拍賣 (賣家)
- `POST /api/v1/auctions/:id:activate` - 啟用拍賣 (賣家)
- `POST /api/v1/auctions/:id:cancel` - 取消拍賣 (賣家/管理員)
- `GET /api/v1/auctions` - 拍賣列表 (公開)
- `GET /api/v1/auctions/:id` - 拍賣詳情 (公開)

### 出價
- `POST /api/v1/auctions/:id/bids` - 提交出價 (買家)
- `GET /api/v1/auctions/:id/my-bids` - 查看我的出價 (買家)
- `GET /api/v1/auctions/:id/results` - 拍賣結果 (前7名/賣家)
- `GET /api/v1/auctions/:id/stats/histogram` - 出價分佈圖

### 管理功能
- `GET /api/v1/admin/blacklist` - 黑名單列表 (管理員)
- `POST /api/v1/admin/blacklist` - 新增黑名單 (管理員)
- `DELETE /api/v1/admin/blacklist/:user_id` - 移除黑名單 (管理員)

### WebSocket 即時功能
- `WS /ws/auctions/:auction_id` - 拍賣房間 WebSocket 連接
- `GET /ws/stats` - WebSocket 連接統計

#### WebSocket 訊息類型
- `hello` - 歡迎訊息 (連接建立時)
- `state` - 拍賣狀態變更 (啟用/取消)
- `bid_accepted` - 出價成功 (即時廣播)
- `extended` - 軟關閉延長 (即時廣播)
- `closed` - 拍賣結束/取消
- `resume_ok` - 斷線恢復確認
- `error` - 錯誤訊息

## 🗃️ 資料庫結構

### 核心表格
- `auctions` - 拍賣主表
- `bids` - 出價記錄
- `auction_status_ref` - 拍賣狀態參考
- `auction_status_history` - 狀態變更歷史
- `auction_events` - 拍賣事件記錄

### 功能表格
- `user_blacklist` - 黑名單
- `auction_bidder_aliases` - 匿名別名映射
- `auction_bid_histograms` - 出價分佈快照
- `auction_notification_log` - 通知記錄
- `audit_logs` - 審計日誌

## 🔧 開發命令

```bash
# 編譯
make build

# 執行
make run

# 清理
make clean

# 依賴管理
make tidy

# 資料庫遷移
make migrate        # 執行遷移
make migrate-down   # 回滾遷移
make migrate-status # 檢查遷移狀態

# 執行結束作業
make finalize-job

# 測試
make test
make test-coverage
```

## 🚢 部署

### Google Cloud Run

```bash
# 設定專案 ID
export GOOGLE_CLOUD_PROJECT=your-project-id

# 部署
./deploy.sh

# 或指定標籤
./deploy.sh v1.0.0
```

### 環境變數設定

生產環境需要在 Google Cloud Console 設定以下 Secret：
- `auction-db-config` (host, user, password, database)
- `auction-redis-config` (host)
- `auction-jwt-config` (secret)

## 📊 監控與日誌

### 健康檢查
- `GET /healthz` - 簡單健康檢查
- `GET /health` - 詳細健康狀態

### 日誌格式
使用 Zap 結構化日誌，包含：
- Request ID 追蹤
- 用戶操作審計
- 錯誤堆疊追蹤
- 性能指標

## 🔒 安全考量

- JWT Token 驗證
- SQL 注入防護 (參數化查詢)
- 出價頻率限制 (5秒/次)
- 黑名單機制
- 敏感資料雜湊處理
- CORS 配置

## 📈 性能優化

- 資料庫索引優化
- 連線池管理
- 軟關閉事務鎖定
- 審計日誌異步寫入

## 🧪 測試

目前尚未實施測試，建議後續加入：
- 單元測試 (業務邏輯)
- 整合測試 (API 端點)
- 壓力測試 (併發出價)

## 📋 待辦事項

### Phase 2 - WebSocket 即時功能 (已實施)
- ✅ WebSocket 連線管理
- ✅ 即時出價廣播
- ✅ 斷線重連機制
- ✅ 軟關閉延長推送
- ✅ 降級控制機制
- ✅ 心跳和連線監控

### Phase 3 - 風控與優化
- [ ] 多層快取策略
- [ ] 降級機制
- [ ] 異常偵測
- [ ] 監控告警

## 📞 支援

如有問題請聯繫開發團隊或查看相關文檔。