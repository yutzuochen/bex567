# Docker Compose Integration Summary

拍賣服務已成功整合到主要的 Docker Compose 配置中。

## 🔧 集成的變更

### 1. 服務架構
現在系統包含以下服務：
- **mysql**: 共享資料庫（支援兩個業務庫）
- **redis**: 共享 Redis（不同 DB 編號分離）
- **backend**: 主要市場平台 (:8080)
- **auction**: 拍賣服務 (:8081)
- **frontend**: 前端服務 (:3000)
- **adminer**: 資料庫管理工具 (:8082)

### 2. 資料庫設定
- **自動建立雙資料庫**: `business_exchange` 和 `auction_service`
- **共享用戶權限**: `app` 用戶可存取兩個資料庫
- **初始化腳本**: `scripts/init-databases.sql`

### 3. 端口配置
```
3000 - Frontend (Next.js)
6379 - Redis
8080 - Main Backend API
8081 - Auction Service API  
8082 - Adminer (was 8081)
3306 - MySQL
```

### 4. 環境變數
```bash
# Frontend 環境變數
NEXT_PUBLIC_API_URL=http://localhost:8080          # 主要 API
NEXT_PUBLIC_AUCTION_API_URL=http://localhost:8081  # 拍賣 API

# Backend 服務
DB_NAME=business_exchange  # 主要業務庫
REDIS_DB=0                 # Redis DB 0

# Auction 服務  
DB_NAME=auction_service    # 拍賣業務庫
REDIS_DB=1                 # Redis DB 1
```

## 🚀 使用方式

### 啟動完整開發環境
```bash
# 啟動所有服務 (含熱重載)
make dev

# 檢查服務狀態
make status

# 查看所有日誌
make logs

# 查看特定服務日誌
make logs-backend   # 主要後端
make logs-auction   # 拍賣服務
make logs-frontend  # 前端
```

### 啟動生產環境
```bash
make up     # 啟動生產堆疊
make down   # 停止所有服務
```

### 清理環境
```bash
make clean    # 清理容器和卷
make rebuild  # 重建所有服務
```

## 🔗 服務訪問

| 服務 | URL | 說明 |
|------|-----|------|
| 主要 API | http://localhost:8080 | 用戶、列表、消息等 |
| 拍賣 API | http://localhost:8081 | 拍賣、出價、WebSocket |
| 前端 | http://localhost:3000 | React 應用 |
| Adminer | http://localhost:8082 | 資料庫管理 |
| WebSocket | ws://localhost:8081/ws/ | 即時拍賣更新 |

## 📋 健康檢查

所有服務都配置了健康檢查：
- MySQL: mysqladmin ping
- Redis: redis-cli ping  
- Backend: /healthz 端點
- Auction: /healthz 端點
- Frontend: /api/healthz 端點

## 🔧 開發工作流程

1. **首次設置**:
   ```bash
   # 複製環境配置文件
   cp business_exchange_marketplace/env.example business_exchange_marketplace/.env
   cp business_exchange_marketplace_auction/env.example business_exchange_marketplace_auction/.env
   ```

2. **啟動開發環境**:
   ```bash
   make dev
   ```

3. **執行數據庫遷移**:
   ```bash
   # 主要服務遷移
   cd business_exchange_marketplace
   make migrate
   
   # 拍賣服務遷移  
   cd ../business_exchange_marketplace_auction
   make migrate
   ```

4. **測試服務**:
   - 主要 API: curl http://localhost:8080/healthz
   - 拍賣 API: curl http://localhost:8081/healthz
   - 前端: curl http://localhost:3000/api/healthz

## 🛠️ 故障排除

### 常見問題

1. **端口衝突**: 確保 8080-8082 和 3000 端口未被占用
2. **資料庫連接**: 等待 MySQL 健康檢查完成
3. **Redis 連接**: 檢查 Redis 是否正常啟動
4. **WebSocket 連接**: 確保拍賣服務正在運行

### 檢查命令
```bash
# 檢查所有服務狀態
docker compose -f docker-compose.dev.yml ps

# 檢查服務日誌
docker compose -f docker-compose.dev.yml logs [service-name]

# 重啟特定服務
docker compose -f docker-compose.dev.yml restart [service-name]
```

## 🔄 服務依賴關係

```
MySQL + Redis (基礎設施)
    ↓
Backend + Auction (API 服務)
    ↓  
Frontend (Web 界面)
```

所有服務都正確配置了依賴關係，確保按正確順序啟動。