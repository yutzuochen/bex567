1. 拍賣規則
- 拍賣類型：密封投標（盲標）
- 拍賣對象：企業主需要主動設定為拍賣模式
- 拍賣開始與結束由企業主自行決定，但不超過61天
- 價格機制：業主會訂一個價格區間，投標者能看到這個區間，並且只能投標在這個區間，否則投標失敗，頁面也會顯示投標失敗:投標價格不在目標區間
- 資格限制：所有會員，進黑名單的會員除外，黑名單判定標準為工作人員手動判斷，並且標記於該玩家中
- 軟關閉邏輯（anti-sniping），結束前 3 分鐘若有人出價，自動延長 1 分鐘
- 匿名化出價者顯示

2. 拍賣結束後
- 價高的前7名會收到通知
- 拍賣狀態更新為 "ended"
- 系統自動發送即時通知給所有參與者
  * 得標者：恭喜得標通知 + 後續步驟說明
  * 賣方：得標結果通知 + 買方聯繫資訊
  * 其他出價者：拍賣結束通知
平台促成聯繫：
- 解除雙方匿名狀態
- 提供雙方聯繫資訊（email, 電話）
- 系統提醒雙方進行初步聯繫確認  

3. 數據庫設計:
- 拍賣狀態表需要包含以下狀態(draft, active, extended, ended, cancelled)
- 出價表需要軟刪除
- 審計日誌表以進行重要操作追蹤
* **盲標**：即時不公開他人出價，只在結束時計算名次（含前 7 名通知）。
* **軟刪除**：`bids.deleted_at`。
* **拍賣狀態**：`draft / active / extended / ended / cancelled`。
* **價格區間限制**：`allowed_min_bid`～`allowed_max_bid`。
* **軟關閉**：最後 3 分鐘內有出價 → 自動延長 1 分鐘（在 `auctions.*_sec` 設定）。
* **黑名單**：全站層級，阻擋投標。
* **分佈圖**：以背景任務寫入 `auction_bid_histograms`。
* **WS 斷線恢復**：`auction_stream_offsets` + `auction_events`。

> 時間欄位建議存 **UTC** 的 `DATETIME`，由應用層轉時區。



### 1) 參考表：拍賣狀態（可調整、不用改 ENUM）

```sql
CREATE TABLE auction_status_ref (
  status_code VARCHAR(16) PRIMARY KEY,
  is_open BOOLEAN NOT NULL,
  description VARCHAR(255) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO auction_status_ref (status_code, is_open, description) VALUES
('draft',     FALSE, 'Not visible / not started'),
('active',    TRUE,  'Running'),
('extended',  TRUE,  'Running with soft-close extension in effect'),
('ended',     FALSE, 'Finished, ranking calculated'),
('cancelled', FALSE, 'Cancelled by seller or admin');
```

### 2) 拍賣主表（盲標／密封投標）

```sql
CREATE TABLE auctions (
  auction_id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  listing_id BIGINT UNSIGNED NOT NULL,        -- 你既有的 listing PK
  seller_id  BIGINT UNSIGNED NOT NULL,        -- users.id
  auction_type ENUM('sealed','english','dutch') NOT NULL DEFAULT 'sealed',
  status_code VARCHAR(16) NOT NULL,
  allowed_min_bid DECIMAL(18,2) NOT NULL,
  allowed_max_bid DECIMAL(18,2) NOT NULL,
  soft_close_trigger_sec INT NOT NULL DEFAULT 180,  -- 結束前3分鐘觸發
  soft_close_extend_sec  INT NOT NULL DEFAULT 60,   -- 延長1分鐘
  start_at   DATETIME NOT NULL,
  end_at     DATETIME NOT NULL,
  extended_until DATETIME NULL,               -- 最近一次延長後的截止時間
  extension_count INT NOT NULL DEFAULT 0,
  is_anonymous BOOLEAN NOT NULL DEFAULT TRUE, -- 盲標 + 匿名
  view_count INT NOT NULL DEFAULT 0,

  -- 「不超過61天」建議由應用層/Job 檢查；下列 CHECK 在 MySQL 8 才會強制
  CONSTRAINT chk_bid_range CHECK (allowed_min_bid >= 0 AND allowed_max_bid > allowed_min_bid),
  CONSTRAINT chk_duration  CHECK (TIMESTAMPDIFF(DAY, start_at, end_at) BETWEEN 1 AND 61),

  CONSTRAINT fk_auction_status
    FOREIGN KEY (status_code) REFERENCES auction_status_ref(status_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_auctions_status_end ON auctions (status_code, end_at);
CREATE INDEX idx_auctions_listing     ON auctions (listing_id);
```

### 3) 出價表（軟刪除、盲標、結束時計名）

```sql
CREATE TABLE bids (
  bid_id     BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  bidder_id  BIGINT UNSIGNED NOT NULL,     -- users.id
  amount     DECIMAL(18,2) NOT NULL,
  client_seq BIGINT NOT NULL,              -- 冪等鍵(同一使用者5分鐘內)
  source_ip_hash VARBINARY(32) NULL,
  user_agent_hash VARBINARY(32) NULL,
  accepted   BOOLEAN NOT NULL DEFAULT TRUE,     -- 不在區間/逾時/黑名單 → FALSE
  reject_reason VARCHAR(64) NULL,               -- 'out_of_range'/'too_late'/...
  final_rank INT NULL,                           -- 拍賣結束後批次寫入（1=最高）
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,                      -- 軟刪除
  deleted_by BIGINT UNSIGNED NULL,

  CONSTRAINT fk_bids_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id),
  CONSTRAINT chk_bid_amount  CHECK (amount >= 0),

  UNIQUE KEY uk_idem (auction_id, bidder_id, client_seq),
  INDEX idx_bids_auction_time (auction_id, created_at),
  INDEX idx_bids_auction_amount (auction_id, amount DESC),
  INDEX idx_bids_auction_rank (auction_id, final_rank)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 4) 拍賣狀態歷史（追蹤 extended/ended 等轉換）

```sql
CREATE TABLE auction_status_history (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  from_status VARCHAR(16) NOT NULL,
  to_status   VARCHAR(16) NOT NULL,
  reason      VARCHAR(255) NULL,
  operator_id BIGINT UNSIGNED NULL,       -- 管理員/系統帳號
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  CONSTRAINT fk_hist_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_hist_auction_time ON auction_status_history (auction_id, created_at);
```

### 5) 事件表（WS 對帳 / 斷線恢復 / 審計）

```sql
CREATE TABLE auction_events (
  event_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,  -- 用於 last_event_id 對帳
  auction_id BIGINT UNSIGNED NOT NULL,
  event_type ENUM('open','bid_accepted','bid_rejected','extended','closed','notified','error') NOT NULL,
  actor_user_id BIGINT UNSIGNED NULL,       -- 出價人或系統
  payload JSON NULL,                        -- 含延長後 end_at、拒絕原因等
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  CONSTRAINT fk_events_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_events_auction_time ON auction_events (auction_id, created_at);
```

### 6) 匿名顯示（每個拍賣給一個代號，如「Bidder #23」）

```sql
CREATE TABLE auction_bidder_aliases (
  auction_id BIGINT UNSIGNED NOT NULL,
  bidder_id  BIGINT UNSIGNED NOT NULL,
  alias_num  INT NOT NULL,                  -- 23
  alias_label VARCHAR(32) NOT NULL,         -- 'Bidder #23'
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY (auction_id, bidder_id),
  UNIQUE KEY uk_alias_label (auction_id, alias_label),
  CONSTRAINT fk_alias_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 7) 出價分佈（價格區間桶／每5分鐘重算一次）

```sql
CREATE TABLE auction_bid_histograms (
  auction_id BIGINT UNSIGNED NOT NULL,
  bucket_low  DECIMAL(18,2) NOT NULL,
  bucket_high DECIMAL(18,2) NOT NULL,
  bid_count   INT NOT NULL,
  computed_at DATETIME NOT NULL,

  PRIMARY KEY (auction_id, bucket_low, bucket_high, computed_at),
  CONSTRAINT fk_histogram_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 8) 黑名單（手動標記；全站有效）

```sql
CREATE TABLE user_blacklist (
  user_id BIGINT UNSIGNED PRIMARY KEY,         -- users.id
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  reason VARCHAR(255) NULL,
  staff_id BIGINT UNSIGNED NULL,               -- 標記人員
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 9) 通知紀錄（結束後：得標者 / 前7名 / 其他）

```sql
CREATE TABLE auction_notification_log (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  kind ENUM('winner','seller_result','top7','participant_end') NOT NULL,
  channel ENUM('email','sms','line','webpush') NOT NULL,
  status ENUM('queued','sent','failed') NOT NULL DEFAULT 'queued',
  meta JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  UNIQUE KEY uk_once (auction_id, user_id, kind),
  INDEX idx_notif_auction (auction_id, created_at),
  CONSTRAINT fk_notif_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 10) WS 斷線恢復（每位用戶在某拍賣讀到的最後事件）

```sql
CREATE TABLE auction_stream_offsets (
  auction_id BIGINT UNSIGNED NOT NULL,
  user_id    BIGINT UNSIGNED NOT NULL,
  last_event_id BIGINT UNSIGNED NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (auction_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 11) 審計日誌（重要操作追蹤）

```sql
CREATE TABLE audit_logs (
  audit_id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  actor_user_id BIGINT UNSIGNED NULL,
  action VARCHAR(64) NOT NULL,               -- 'AUCTION_CREATE','BID_PLACE','AUCTION_EXTEND','AUCTION_CLOSE',...
  entity_type VARCHAR(32) NOT NULL,          -- 'auction','bid','user','blacklist',...
  entity_id   BIGINT UNSIGNED NOT NULL,
  before_state JSON NULL,
  after_state  JSON NULL,
  ip VARBINARY(16) NULL,
  user_agent_hash VARBINARY(32) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  INDEX idx_audit_entity (entity_type, entity_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```


## 重要設計對應你的需求

* **軟關閉（anti-sniping）**：在受理出價時，若 `end_at - NOW() <= soft_close_trigger_sec`，就把 `extended_until = end_at + soft_close_extend_sec`、`extension_count += 1`，同時寫入 `auction_status_history` 與 `auction_events('extended')`，`auctions.status_code` 可暫設為 `extended`（或仍維持 `active`，視前端顯示策略）。
* **盲標 / 區間驗證**：受理時檢查 `amount BETWEEN allowed_min_bid AND allowed_max_bid`；不符合 → `accepted = FALSE, reject_reason='out_of_range'`，並寫 `auction_events('bid_rejected')`。
* **終局排名**：拍賣結束 Job 對 `bids.accepted=TRUE AND deleted_at IS NULL` 依 `amount DESC, created_at ASC` 排序，回寫 `bids.final_rank`；`final_rank IN (1..7)` 觸發 `auction_notification_log`（得標者、前7名、其他參與者）。
* **匿名化**：頁面顯示 `auction_bidder_aliases.alias_label`；別把 `bidder_id` 直接回傳。
* **黑名單**：出價前先查 `user_blacklist.is_active=TRUE` → 直接拒絕並記 `auction_events('bid_rejected')`。
* **軟刪除**：管理端撤銷可寫 `bids.deleted_at`，查詢皆需 `... AND deleted_at IS NULL`。
* **分佈圖**：每 5 分鐘批次把當前區間桶計算寫入 `auction_bid_histograms`，供前端圖表讀取（避免熱路徑 heavy scan）。


## 查詢與索引建議（片段）

* **即將截止列表**：

```sql
SELECT auction_id, listing_id, end_at
FROM auctions
WHERE status_code IN ('active','extended')
ORDER BY end_at ASC
LIMIT 50;
```

* **計算前 7 名（結束時計算一次即可）**：

```sql
SELECT bid_id, bidder_id, amount
FROM bids
WHERE auction_id = ? AND accepted = TRUE AND deleted_at IS NULL
ORDER BY amount DESC, created_at ASC
LIMIT 7;
```

* **當前用於 WS 對帳的事件拉取**：

```sql
SELECT event_id, event_type, payload, created_at
FROM auction_events
WHERE auction_id = ? AND event_id > ?
ORDER BY event_id ASC
LIMIT 500;
```

- migration
0001_create_auction_core_tables.up.sql
-- 核心資料表（拍賣、出價、狀態歷史、事件、匿名映射、分佈、黑名單、通知、串流偏移、審計）
-- 目標：MySQL 8.x / InnoDB / utf8mb4

-- 1) 參考狀態表
CREATE TABLE IF NOT EXISTS auction_status_ref (
  status_code VARCHAR(16) PRIMARY KEY,
  is_open BOOLEAN NOT NULL,
  description VARCHAR(255) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 2) 拍賣主表（密封投標為預設）
CREATE TABLE IF NOT EXISTS auctions (
  auction_id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  listing_id BIGINT UNSIGNED NOT NULL,
  seller_id  BIGINT UNSIGNED NOT NULL,
  auction_type ENUM('sealed','english','dutch') NOT NULL DEFAULT 'sealed',
  status_code VARCHAR(16) NOT NULL,
  allowed_min_bid DECIMAL(18,2) NOT NULL,
  allowed_max_bid DECIMAL(18,2) NOT NULL,
  soft_close_trigger_sec INT NOT NULL DEFAULT 180,  -- 結束前 3 分鐘觸發
  soft_close_extend_sec  INT NOT NULL DEFAULT 60,   -- 延長 1 分鐘
  start_at   DATETIME NOT NULL,
  end_at     DATETIME NOT NULL,
  extended_until DATETIME NULL,
  extension_count INT NOT NULL DEFAULT 0,
  is_anonymous BOOLEAN NOT NULL DEFAULT TRUE,
  view_count INT NOT NULL DEFAULT 0,

  CONSTRAINT chk_bid_range CHECK (allowed_min_bid >= 0 AND allowed_max_bid > allowed_min_bid),
  CONSTRAINT chk_duration  CHECK (TIMESTAMPDIFF(DAY, start_at, end_at) BETWEEN 1 AND 61),

  CONSTRAINT fk_auction_status
    FOREIGN KEY (status_code) REFERENCES auction_status_ref(status_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_auctions_status_end ON auctions (status_code, end_at);
CREATE INDEX idx_auctions_listing     ON auctions (listing_id);
CREATE INDEX idx_auctions_seller      ON auctions (seller_id);

-- 3) 出價表（盲標、軟刪除、結束時計名）
CREATE TABLE IF NOT EXISTS bids (
  bid_id     BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  bidder_id  BIGINT UNSIGNED NOT NULL,
  amount     DECIMAL(18,2) NOT NULL,
  client_seq BIGINT NOT NULL,
  source_ip_hash VARBINARY(32) NULL,
  user_agent_hash VARBINARY(32) NULL,
  accepted   BOOLEAN NOT NULL DEFAULT TRUE,
  reject_reason VARCHAR(64) NULL,            -- 'out_of_range'/'too_late'/'blacklisted'/...
  final_rank INT NULL,                        -- 拍賣結束後寫入
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  deleted_by BIGINT UNSIGNED NULL,

  CONSTRAINT fk_bids_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id),
  CONSTRAINT chk_bid_amount  CHECK (amount >= 0),

  UNIQUE KEY uk_idem (auction_id, bidder_id, client_seq),
  INDEX idx_bids_user (bidder_id, auction_id, created_at),
  INDEX idx_bids_auction_time (auction_id, created_at),
  INDEX idx_bids_auction_amount (auction_id, amount DESC),
  INDEX idx_bids_auction_rank (auction_id, final_rank)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 4) 拍賣狀態歷史
CREATE TABLE IF NOT EXISTS auction_status_history (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  from_status VARCHAR(16) NOT NULL,
  to_status   VARCHAR(16) NOT NULL,
  reason      VARCHAR(255) NULL,
  operator_id BIGINT UNSIGNED NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  CONSTRAINT fk_hist_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_hist_auction_time ON auction_status_history (auction_id, created_at);

-- 5) 拍賣事件（WS 對帳、斷線恢復、審計）
CREATE TABLE IF NOT EXISTS auction_events (
  event_id   BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  event_type ENUM('open','bid_accepted','bid_rejected','extended','closed','notified','error') NOT NULL,
  actor_user_id BIGINT UNSIGNED NULL,
  payload JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  CONSTRAINT fk_events_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_events_auction_time ON auction_events (auction_id, created_at);

-- 6) 匿名別名（Bidder #23）
CREATE TABLE IF NOT EXISTS auction_bidder_aliases (
  auction_id BIGINT UNSIGNED NOT NULL,
  bidder_id  BIGINT UNSIGNED NOT NULL,
  alias_num  INT NOT NULL,
  alias_label VARCHAR(32) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY (auction_id, bidder_id),
  UNIQUE KEY uk_alias_label (auction_id, alias_label),
  CONSTRAINT fk_alias_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 7) 出價分佈快照（背景任務每 5 分鐘寫一次）
CREATE TABLE IF NOT EXISTS auction_bid_histograms (
  auction_id BIGINT UNSIGNED NOT NULL,
  bucket_low  DECIMAL(18,2) NOT NULL,
  bucket_high DECIMAL(18,2) NOT NULL,
  bid_count   INT NOT NULL,
  computed_at DATETIME NOT NULL,

  PRIMARY KEY (auction_id, bucket_low, bucket_high, computed_at),
  CONSTRAINT fk_histogram_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 8) 黑名單（全站）
CREATE TABLE IF NOT EXISTS user_blacklist (
  user_id BIGINT UNSIGNED PRIMARY KEY,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  reason VARCHAR(255) NULL,
  staff_id BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 9) 通知紀錄（得標者/前7名/參與者/賣家）
CREATE TABLE IF NOT EXISTS auction_notification_log (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  auction_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  kind ENUM('winner','seller_result','top7','participant_end') NOT NULL,
  channel ENUM('email','sms','line','webpush') NOT NULL,
  status ENUM('queued','sent','failed') NOT NULL DEFAULT 'queued',
  meta JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  UNIQUE KEY uk_once (auction_id, user_id, kind),
  INDEX idx_notif_auction (auction_id, created_at),
  CONSTRAINT fk_notif_auction FOREIGN KEY (auction_id) REFERENCES auctions(auction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 10) WS 斷線恢復（每用戶最後讀到的事件）
CREATE TABLE IF NOT EXISTS auction_stream_offsets (
  auction_id BIGINT UNSIGNED NOT NULL,
  user_id    BIGINT UNSIGNED NOT NULL,
  last_event_id BIGINT UNSIGNED NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (auction_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 11) 審計日誌
CREATE TABLE IF NOT EXISTS audit_logs (
  audit_id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  actor_user_id BIGINT UNSIGNED NULL,
  action VARCHAR(64) NOT NULL,               -- 'AUCTION_CREATE','BID_PLACE',...
  entity_type VARCHAR(32) NOT NULL,          -- 'auction','bid','user','blacklist',...
  entity_id   BIGINT UNSIGNED NOT NULL,
  before_state JSON NULL,
  after_state  JSON NULL,
  ip VARBINARY(16) NULL,
  user_agent_hash VARBINARY(32) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  INDEX idx_audit_entity (entity_type, entity_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

0001_create_auction_core_tables.down.sql
-- 反向刪除，按外鍵相依順序
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS auction_stream_offsets;
DROP TABLE IF EXISTS auction_notification_log;
DROP TABLE IF EXISTS user_blacklist;
DROP TABLE IF EXISTS auction_bid_histograms;
DROP TABLE IF EXISTS auction_bidder_aliases;
DROP TABLE IF EXISTS auction_events;
DROP TABLE IF EXISTS auction_status_history;
DROP TABLE IF EXISTS bids;
DROP TABLE IF EXISTS auctions;
DROP TABLE IF EXISTS auction_status_ref;

0002_seed_statuses.up.sql
-- 預置拍賣狀態
INSERT INTO auction_status_ref (status_code, is_open, description) VALUES
('draft',     FALSE, 'Not visible / not started'),
('active',    TRUE,  'Running'),
('extended',  TRUE,  'Running with soft-close extension in effect'),
('ended',     FALSE, 'Finished, ranking calculated'),
('cancelled', FALSE, 'Cancelled by seller or admin')
ON DUPLICATE KEY UPDATE
  is_open = VALUES(is_open),
  description = VALUES(description);

0002_seed_statuses.down.sql
DELETE FROM auction_status_ref
WHERE status_code IN ('draft','active','extended','ended','cancelled');

可選（如果你想把索引分開管理）

上面 0001 已含索引。若你偏好把索引獨立一檔，可再拆 0003_add_indexes.up.sql，但不是必須。



先在 測試專案 跑 0001 → 0002，確認：

CHECK 約束在你 Cloud SQL 版本會被強制（MySQL 8.0.16+）

既有 users / listings 的外鍵是否需要加（我先不加，以免破壞既有表；要加告訴我欄位型別/名稱我補上）

我的盲標邏輯、軟關閉、黑名單、匿名顯示 都已對應：

區間：allowed_min_bid ~ allowed_max_bid（應用層校驗 + 拒絕寫 bids.accepted=FALSE）

軟關閉：應用層更新 extended_until、extension_count，並在 auction_status_history/auction_events 記錄

黑名單：查 user_blacklist 後決定是否接受

匿名：用 auction_bidder_aliases 對照


後端 API:
- 使用 Golang
- 代碼放在 business_exchange_marketplace_auction 中，之後會在 Cloud Run 上新增一個 instance


---

# 線上概覽

* Base URL：`/api/v1`
* 認證：JWT（`Authorization: Bearer <token>`），角色 `buyer | seller | admin`
* Content-Type：`application/json; charset=utf-8`
* 時間：一律 **UTC** `RFC3339`
* 貨幣：`TWD`，金額 `DECIMAL(18,2)`
* 版本：URL 版控（`/v1`）

---

# 狀態機（State Machine）

`draft → active → (active|extended)* → ended | cancelled`

* **extended**：在「結束前 3 分鐘」收到任何有效出價 → **延長 1 分鐘**，可多次觸發。
* **ended**：到期自動結束或管理員/賣家結束 → 計算名次、發送通知（得標者、前 7 名、參與者、賣家）。

---

# 核心驗證規則（摘要）

* 拍賣期間 ≤ **61 天**
* 出價金額需落在 **\[allowed\_min\_bid, allowed\_max\_bid]**（含邊界）
* 任何會員可出價；**黑名單**不可（`403 blacklisted`）
* **密封投標**：進行中不可讀取他人出價明細（僅回自己出價與**聚合分佈**）
* **軟關閉**：`end_at - now <= 180s` 時有有效出價 → `end_at += 60s`（並廣播 `extended`）

---

# REST API（關鍵端點）

## Auctions（賣家/管理）

### 建立拍賣（賣家）

`POST /auctions`

```json
{
  "listing_id": 123,
  "allowed_min_bid": 5000000,
  "allowed_max_bid": 9000000,
  "start_at": "2025-09-01T02:00:00Z",
  "end_at":   "2025-09-20T02:00:00Z",
  "is_anonymous": true
}
```

**201**

```json
{
  "auction_id": 987,
  "status_code": "draft",
  "soft_close_trigger_sec": 180,
  "soft_close_extend_sec": 60
}
```

**422**：`duration_exceeded | invalid_range | start_in_past`

### 啟用拍賣（賣家）

`POST /auctions/{auction_id}:activate` → **draft → active**
**200**：`{"status_code":"active","start_at":"...","end_at":"..."}`
**409**：`invalid_state`

### 取消拍賣（賣家/管理）

`POST /auctions/{auction_id}:cancel`（僅 `draft|active|extended` 可取消）
**200**：`{"status_code":"cancelled"}`

### 取得拍賣列表（公開）

`GET /auctions?status=active&city=Taipei&industry=food-service&sort=end_at&limit=20&page_token=...`
**200**

```json
{
  "items": [{
    "auction_id":987,
    "listing_id":123,
    "status_code":"active",
    "start_at":"..","end_at":"..",
    "allowed_min_bid":5000000,
    "allowed_max_bid":9000000,
    "is_anonymous":true,
    "extended_until":"2025-09-20T02:05:00Z",
    "extension_count":2,
    "stats":{"participants":42}
  }],
  "next_page_token":"..."
}
```

### 取得單一拍賣（公開）

`GET /auctions/{auction_id}`
**200**

```json
{
  "auction":{
    "auction_id":987,"listing_id":123,"status_code":"active",
    "start_at":"...","end_at":"...","extended_until":"...","extension_count":2,
    "allowed_min_bid":5000000,"allowed_max_bid":9000000,"is_anonymous":true
  },
  "viewer":{
    "can_bid": true,                          // 已登錄且非黑名單
    "alias_label": "Bidder #23",              // 若已曾入場/出價
    "blacklisted": false
  }
}
```

### 取得出價分佈（聚合；不暴露個別出價）

`GET /auctions/{auction_id}/stats/histogram?at=2025-09-01T02:10:00Z`
**200**

```json
{
  "computed_at":"2025-09-01T02:10:00Z",
  "buckets":[
    {"low":5000000,"high":5600000,"count":8},
    {"low":5600000,"high":6200000,"count":5}
  ],
  "k_anonymity_min":5
}
```

> 備註：僅回 `count >= 5` 的桶，避免資訊洩漏。

## Bids（買家）

### 提交出價（密封投標）

`POST /auctions/{auction_id}/bids`
Headers：`X-Idempotency-Key: <uuid>`（或 body 的 `client_seq`）

```json
{ "amount": 6200000, "client_seq": 172501234 }
```

**200**（接受或拒絕，密封不回傳他人資訊）

```json
{
  "accepted": true,
  "reject_reason": null,
  "server_time":"2025-09-01T02:11:03Z",
  "soft_close": {"extended": true, "extended_until": "2025-09-01T02:12:03Z"},
  "event_id": 45678
}
```

可能錯誤：

* **403** `blacklisted`
* **409** `auction_closed | auction_not_active | out_of_range | too_frequent | soft_deleted | past_deadline`
* **422** `invalid_amount`
* **429** `rate_limited`

### 查詢「我在此拍賣的出價」

`GET /auctions/{auction_id}/my-bids?limit=20&page_token=...`
**200**

```json
{
  "items":[
    {"bid_id":1,"amount":6200000,"accepted":true,"created_at":"..."},
    {"bid_id":2,"amount":6400000,"accepted":false,"reject_reason":"out_of_range","created_at":"..."}
  ],
  "next_page_token":null
}
```

> 進行中僅可看**自己**的出價；`ended` 後賣家/管理可讀所有出價。

### 結束後讀取排名（賣家/管理）

`GET /auctions/{auction_id}/results?limit=50&page_token=...`
**200**

```json
{
  "items":[
    {"final_rank":1,"bidder_alias":"Bidder #78","amount":8800000},
    {"final_rank":2,"bidder_alias":"Bidder #23","amount":8700000}
  ],
  "top_k":7
}
```

## Blacklist（管理）

* `GET /admin/blacklist?active=true&limit=50`
* `POST /admin/blacklist` `{ "user_id": 456, "reason":"fraud-suspected" }`
* `DELETE /admin/blacklist/{user_id}`（或 `PATCH` 變更 `is_active`）

## 控制/維護（管理）

* `POST /admin/auctions/{auction_id}:finalize`：強制結束 & 計名次 & 觸發通知
* `POST /admin/auctions/{auction_id}:recompute-histogram`：重算分佈
* `GET /admin/auctions/{auction_id}/bids`：完整投標明細（僅 `ended`）

---

# WebSocket 規格（即時/斷線恢復）

**連線**：`GET /ws/auctions/{auction_id}?token=JWT&last_event_id=12345`

* 鑑權：JWT（或 `Sec-WebSocket-Protocol: bearer,<JWT>`）
* 心跳：`ping/pong` 每 25s（降級時可調長）

**連線成功（Server → Client）**

```json
{ "type":"hello",
  "server_time":"2025-09-01T02:00:00Z",
  "status_code":"active",
  "end_at":"2025-09-20T02:00:00Z",
  "extended_until":"2025-09-20T02:05:00Z",
  "alias_label":"Bidder #23",
  "can_bid": true,
  "last_event_id": 12345,
  "degraded_level": 0
}
```

**客戶端出價（Client → Server）**

```json
{ "type":"place_bid", "amount": 6400000, "client_seq": 172501234 }
```

**結果（Server → Client）**

* 回給出價者：

```json
{ "type":"bid_accepted", "event_id":45678, "server_time":"...", 
  "soft_close":{"extended":true,"extended_until":"..."} }
```

* 廣播給「其他人」（密封 → **不含金額**，僅延長與狀態）

```json
{ "type":"extended", "event_id":45679, "extended_until":"..." }
```

* 拒絕：

```json
{ "type":"bid_rejected", "event_id":45680, "reason":"out_of_range|blacklisted|auction_closed|too_frequent|invalid_amount" }
```

**狀態推送**

```json
{ "type":"state", "event_id":45681, "status_code":"extended", "end_at":"...", "extended_until":"..." }
```

**結束通知**

```json
{ "type":"closed", "event_id":46000, "ended_at":"...", "next":"results_pending" }
```

**斷線恢復（Client → Server）**

```json
{ "type":"resume", "last_event_id": 45670 }
```

**恢復應答（Server → Client）**

```json
{ "type":"resume_ok", "missed":[ /* 45671..last */ ], "server_time":"..." }
```

> **密封投標限制**：進行中**不會**廣播任何「金額」或「出價者識別」，只發送 `extended/state/closed` 這類事件；出價者**自己**會收到 `bid_accepted/bid_rejected`。

---

# 錯誤碼（HTTP + 業務代碼）

* 400：`bad_request`
* 401：`unauthorized`
* 403：`blacklisted | forbidden`
* 404：`not_found`
* 409：`auction_not_active | auction_closed | out_of_range | too_frequent | past_deadline | invalid_state | soft_deleted`
* 422：`invalid_amount | invalid_range | duration_exceeded`
* 429：`rate_limited`
* 503：`service_unavailable | degraded_mode`

> 回應格式（統一）

```json
{ "error": { "code": "out_of_range", "message": "Bid must be within target range", "hint": "allowed_min_bid=5,000,000 ~ allowed_max_bid=9,000,000" } }
```

---

# 併發控制與一致性（關鍵實作提示）

* **寫路徑**：單行鎖或樂觀鎖（建議 `SELECT ... FOR UPDATE` 鎖 `auctions` 行；或 Redis 鎖 `auction:{id}` ≤ 200ms）
* **軟關閉**：受理有效出價後，若 `end_at - now <= 180s` → `end_at = end_at + 60s`（更新 `extended_until` 與 `extension_count`），寫入 `auction_events('extended')`
* **冪等**：`X-Idempotency-Key` 或 `client_seq` + `(auction_id, bidder_id)` 組合唯一 5 分鐘
* **去重**：同一用戶同一拍賣 5 秒內限 1 次（HTTP 409 `too_frequent`）
* **時鐘**：所有邏輯以伺服器時間為準；每個回應含 `server_time`，WS `hello/state` 也帶
* **密封**：進行中不得提供他人出價明細（API 與 WS 都限制）

---

# 高併發降級信號

* 伺服器在 WS `hello/state` 內回 `degraded_level: 0|1|2|3|4`

  * L1：提高快取 TTL、心跳 25→40s
  * L2：出價節流（5s/次）、直方圖/人數更新改 5s 才推
  * L3：停用「即時歷史」、只保留出價受理與延長事件
  * L4：啟用「出價排隊」、HTTP 輪詢備援

---

# 背景 Job / Webhook

* **Finalize Job**（到期後自動）

  1. 鎖定拍賣 → 過濾 `accepted=true AND deleted_at IS NULL` → 依 `amount DESC, created_at ASC` 排序
  2. 回寫 `final_rank`，標記 `status=ended`
  3. 產生通知：`winner`（第 1 名）、`top7`（前 7 名）、`participant_end`（其餘）、`seller_result`（賣家）
  4. `auction_events('closed')`
* **Webhook（可選，對內部服務）**

  * `auction.closed`、`auction.extended`、`bid.accepted`、`bid.rejected`、`auction.notified`
  * HMAC 簽章頭：`X-Signature: sha256=...`

---

# 節流（Rate Limits）

* 通用：**30 req/min/IP**（可因登入/角色調整）
* 提交出價：**12 req/min/User**（同拍賣**5 秒 1 次**）
* WS 連線：每帳號並發 **≤ 3**、每 IP **≤ 10**

---

# 快取策略（與你規劃一致）

* **L1 應用快取**（30s）：拍賣基本資訊、參與者統計
* **L2 Redis**（5m）：拍賣詳情、最近 50 筆出價（只給自己）、分佈快照
* **L3 DB**：全量歷史
* **最後 10 分鐘**：停用 L1，直讀 L2；提高 WS 推送頻率；優先處理該拍賣

---

# 安全

* SQL 參數化、輸入白名單（`amount` ≥ 0 且 ≤ `allowed_max_bid`）
* JWT 過期與撤銷（黑名單 Token）
* CSRF：若走 Cookie，同步 `SameSite=Lax` + CSRF Token；建議走 Bearer Token
* DDoS：WAF + IP/帳號節流；異常模式啟用 L3/L4
* 審計：管理/關鍵操作一律記 `audit_logs`

---

# OpenAPI（摘要片段，可放 `openapi.yaml`）

```yaml
openapi: 3.1.0
info:
  title: Auction Service API
  version: 1.0.0
servers: [{ url: /api/v1 }]
components:
  securitySchemes:
    bearerAuth: { type: http, scheme: bearer, bearerFormat: JWT }
  schemas:
    Error:
      type: object
      properties:
        error:
          type: object
          properties:
            code: { type: string }
            message: { type: string }
paths:
  /auctions:
    get:
      security: []
      summary: List auctions
      parameters:
        - in: query; name: status; schema: { type: string, enum: [draft,active,extended,ended,cancelled] }
        - in: query; name: limit; schema: { type: integer, default: 20, maximum: 100 }
        - in: query; name: page_token; schema: { type: string }
      responses:
        '200': { description: OK }
    post:
      security: [ { bearerAuth: [] } ]
      summary: Create auction (seller)
      responses: { '201': { description: Created }, '422': { $ref: '#/components/schemas/Error' } }
  /auctions/{auction_id}:
    get:
      security: []
      summary: Get single auction
      parameters: [ { in: path, name: auction_id, required: true, schema: { type: integer } } ]
      responses: { '200': { description: OK }, '404': { $ref: '#/components/schemas/Error' } }
  /auctions/{auction_id}:activate:
    post:
      security: [ { bearerAuth: [] } ]
      summary: Activate auction
      responses: { '200': { description: OK }, '409': { $ref: '#/components/schemas/Error' } }
  /auctions/{auction_id}/bids:
    post:
      security: [ { bearerAuth: [] } ]
      summary: Place sealed bid
      parameters:
        - in: header; name: X-Idempotency-Key; required: false; schema: { type: string }
      responses:
        '200': { description: OK }
        '403': { $ref: '#/components/schemas/Error' }
        '409': { $ref: '#/components/schemas/Error' }
        '422': { $ref: '#/components/schemas/Error' }
```

---

# 典型流程（順序）

**出價（密封 + 軟關閉）**

1. `POST /auctions/{id}/bids` 驗證（黑名單 → 403；區間外 → 409 out\_of\_range）
2. DB/Redis 鎖 → 檢查 `now < end_at(or extended_until)`
3. 若 `end_at - now <= 180s` → `end_at += 60s`、記 `auction_events('extended')`、WS 廣播 `extended`
4. 寫 `bids(accepted=true)`、記 `auction_events('bid_accepted')`（只回出價者）
5. 回應 `200 accepted=true` + `server_time` + `extended_until`

**結束**

1. Cron/Cloud Scheduler 觸發 Finalize Job
2. 排序、寫 `final_rank`、`status=ended`、事件 `closed`
3. 寄發通知：`winner`、`top7`、`participant_end`、`seller_result`
4. （可選）產出「結果摘要」PDF 或內頁顯示

---



前端頁面:
- 使用 Next.js
- 拍賣專區列表頁
- 出價面板組件

雲端佈署
- 代碼佈署使用 Cloud Run 
- 數據庫佈署使用 Cloud SQL

4. 明確技術需求

需要的技術組件:
- WebSocket 實時通訊
- 軟關閉邏輯（anti-sniping），結束前 3 分鐘若有人出價，自動延長 1 分鐘
4-1 WebSocket 斷線重連機制

客戶端策略

// 重連策略：指數退避 + 最大重試次數
const reconnectConfig = {
maxRetries: 10,
baseDelay: 1000,      // 1秒
maxDelay: 30000,      // 最大30秒
backoffFactor: 1.5
}

// 斷線後自動重連邏輯
- 檢測斷線 → 立即嘗試重連
- 重連失敗 → 等待 1s → 再次重連
- 持續失敗 → 等待時間遞增（1s, 1.5s, 2.25s...）
- 重連成功 → 同步拍賣狀態（防止遺漏出價）

服務端支援

// 斷線恢復時需要提供的資訊
- 用戶最後接收的訊息ID
- 該拍賣的最新狀態
- 遺漏的出價記錄
- 當前伺服器時間校正

4-2. 高併發降級策略

分層降級

Level 1 (輕微負載)：
- 啟用更積極的快取策略
- 延長 WebSocket 心跳間隔

Level 2 (中等負載)：
- 限制單用戶出價頻率（5秒內最多1次）
- 降低即時資料更新頻率

Level 3 (高負載)：
- 停用非核心功能（出價歷史即時更新）
- 僅保留核心出價功能
- 增加出價處理的隊列緩衝

Level 4 (極限負載)：
- 啟用出價排隊機制
- 顯示系統繁忙提示

熔斷機制

- Redis 連線失敗 → 降級到資料庫直接存取
- 資料庫慢查詢 → 返回快取資料 + 降級提示
- WebSocket 服務異常 → 降級到 HTTP 輪詢模式

4-3. 快取策略

多層快取架構

L1 - 應用內快取 (30秒)：
- 拍賣基本資訊
- 當前最高出價
- 參與人數統計

L2 - Redis 快取 (5分鐘)：
- 拍賣詳細資訊
- 最近50筆出價記錄
- 價格區間分布圖資料

L3 - 資料庫 (持久化)：
- 完整出價歷史
- 拍賣完整記錄

快取更新策略

寫入時：
- 先更新資料庫
- 再更新 Redis 快取
- 最後推送 WebSocket 更新

讀取時：
- 優先讀取應用快取
- 快取未命中 → 讀取 Redis
- Redis 未命中 → 讀取資料庫

特殊考慮

拍賣即將結束時 (最後10分鐘)：
- 停用應用快取，直接讀 Redis
- 增加 WebSocket 推送頻率
- 優先處理該拍賣的所有請求

價格分布圖：
- 每5分鐘重新計算一次
- 背景任務更新，避免影響出價響應
- 使用獨立的快取鍵


5.UI
- 左側：標的核心資訊（行業、城市、SDE、毛利、租約剩餘月數、員工數）
- 右側：出價面板（即時 WebSocket）、出價歷史（匿名化：Bidder #23）
- Q&A 區（平台內串訊）
- UI 頁面會顯示各個價格區間的競標筆數，並畫出分佈圖


6.併發與一致性

- 「拍賣專區」做成一個可擴充的競標系統，同時兼顧信任、風控與高併發

- 寫路徑：以單行更新 + 版本號/樂觀鎖為主；或以 Redis 分布式鎖 auction:{id} 包裹出價流程（鎖超時 ≤ 200ms）

- 去重：client_seq + bidder_id 作為 5 分鐘內冪等鍵（防連點/重送）

- 時鐘：以伺服器時鐘為準；向客端推送伺服器時間偏移量（避免用戶端時差誤判）

- 延長判斷：以寫入成功的伺服器時間為基準（不是客戶端時間）

7.安全性
- SQL Injection 防護
- 出價金額驗證 (防止負數、超大數字)
- Rate Limiting (防 DDoS)
- JWT Token 過期處理
- 敏感操作的 CSRF 防護

8. 錯誤處理和用戶體驗

---

# 1) 網路中斷時的用戶提示（WS / HTTP）

### 行為規則

* **即時偵測**：WS `close` 或 `pong` 超時 → 立刻切換為 *離線狀態*。
* **漸進式提示**：3 個層級

  1. **微提示（<1.5s）**：右上角輕量 toast「連線不穩定…」
  2. **固定橫幅（≥1.5s）**：頁面頂部紅色/橘色 banner「與伺服器失去連線，正在嘗試重連…」＋倒數顯示**伺服器時間差**（避免用戶看錯倒數）。
  3. **互動限制**：**停用出價按鈕**（disabled），顯示「離線中，無法送出出價」。
* **自動重連**：用你定的指數退避（1s→1.5s→2.25s… 最多 10 次），攜帶 `last_event_id` 做 `resume`。
* **成功恢復**：綠色 toast「已重新連上，狀態已同步」，橫幅自動收起。

### 建議文案（zh-TW）

* 微提示：**「連線不穩定… 正在重新連線」**
* 橫幅（含行動）：**「與伺服器失去連線，正在嘗試重連（第 3/10 次）。在離線期間無法送出出價。」**
* 恢復：**「已重新連線，狀態已同步」**

### 前端小骨架（React）

```ts
// useConnectivity.ts
export function useConnectivity(ws: WebSocket | null) {
  const [online, setOnline] = useState(true);
  const [retries, setRetries] = useState(0);
  const [reconnecting, setReconnecting] = useState(false);

  useEffect(() => {
    const onDown = () => setOnline(false);
    const onUp = () => setOnline(true);
    window.addEventListener('offline', onDown);
    window.addEventListener('online', onUp);
    return () => { window.removeEventListener('offline', onDown); window.removeEventListener('online', onUp); };
  }, []);

  useEffect(() => {
    if (!ws) return;
    const timer = setInterval(() => {
      if (ws.readyState !== WebSocket.OPEN) { setReconnecting(true); setRetries(r => r + 1); }
      else { setReconnecting(false); setRetries(0); }
    }, 1500);
    return () => clearInterval(timer);
  }, [ws]);

  return { online, reconnecting, retries };
}
```

---

# 2) 出價失敗的具體錯誤訊息（可執行、可修正）

### 原則

* **密封投標**：不暴露他人任何資訊；錯誤只描述「你的出價」為何被拒。
* **立刻可修正**：每個錯誤給**原因 + 建議行動**；能自動修正的提供**一鍵重試**。
* **保持伺服器權威**：送出後顯示「等待伺服器確認…」spinner（最多 3–5s），收到 `bid_accepted` 才成功。

### 錯誤碼 → 使用者文案與動作

| 代碼                                    | 使用者訊息                                        | 建議行動（按鈕/交互）          |
| ------------------------------------- | -------------------------------------------- | -------------------- |
| `out_of_range`                        | **投標失敗：價格不在允許區間**（NT\$5,000,000 – 9,000,000） | 將金額調整到區間內 →「重新出價」    |
| `invalid_amount`                      | **金額格式不正確**（僅允許整數或兩位小數）                      | 更正格式 →「重新出價」         |
| `too_frequent`                        | **出價過於頻繁**，請在 **{cooldown}s** 後再試            | 顯示倒數，按鈕 disabled     |
| `blacklisted`                         | **帳號目前無法參與出價**                               | 「聯繫客服」；不顯示細節         |
| `auction_closed`                      | **拍賣已結束**（{ended\_at\_tz}）                   | 關閉面板                 |
| `auction_not_active`                  | **拍賣尚未開放或已暫停**                               | 返回列表                 |
| `past_deadline`                       | **已超過截止時間**                                  | 關閉面板                 |
| `rate_limited`                        | **系統繁忙，已限制頻率**                               | 2–5s 後自動重試或手動重試      |
| `service_unavailable`/`degraded_mode` | **系統忙碌中，已切換排隊模式**                            | 顯示「排隊中…」；保留請求直至確認或超時 |
| `duplicate`（你可加）                      | **重複提交**，我們已收到前一次出價                          | 關閉提示                 |

### 前端對應（Map）

```ts
const BID_ERROR_COPY: Record<string,(ctx:any)=>{title:string;desc?:string;cta?:string}> = {
  out_of_range: ({min,max}) => ({ title: "投標失敗：價格不在允許區間", desc: `請輸入 NT$${min} – ${max}` , cta: "重新出價"}),
  invalid_amount: () => ({ title: "金額格式不正確", desc: "僅允許整數或兩位小數", cta: "重新出價" }),
  too_frequent: ({cooldown}) => ({ title: "出價過於頻繁", desc: `請在 ${cooldown}s 後再試` }),
  blacklisted: () => ({ title: "帳號目前無法參與出價", desc: "如有疑問請聯繫客服" }),
  auction_closed: ({endedAt}) => ({ title: "拍賣已結束", desc: endedAt }),
  rate_limited: () => ({ title: "系統繁忙，請稍後重試" }),
  service_unavailable: () => ({ title: "系統忙碌中，已切換排隊模式" }),
};
```

---

# 3) 系統維護時的公告機制（可營運、可預告、可讀取）

### 後端配置

* 新增**系統公告**資料結構（或表）：`announcements(id, level[info|maintenance|incident], message, starts_at, ends_at, links[], locale, created_by)`
* API：

  * `GET /status` → `{ healthy, degraded_level, now }`
  * `GET /announcements?active=true&locale=zh-TW` → 回傳**目前生效**的公告（可有多條，按優先排序）
  * WebSocket `hello/state` 內**同步回推** `degraded_level` 與最高優先公告摘要（避免只靠輪詢）

### 前端顯示策略

* **預告維護**（≥24h）：藍色 banner + 倒數（24h/1h/15m 門檻再提醒一次）
* **維護中**：橘色/紅色 banner **「只讀模式」**：瀏覽允許、**出價鍵 disabled**，顯示下一次可出價時間
* **重大事件**（事故）：全站頂部固定紅色 banner + 狀態頁連結
* **多語系**：公告支援 `locale`；預設跟使用者語言

### 文案樣板

* 預告：**「系統將於 9/10 02:00–03:00 進行維護。期間拍賣僅可瀏覽，暫停出價。」**
* 維護中：**「系統維護中（預計 03:00 結束）。目前為只讀模式，出價功能暫停。」**
* 事故：**「系統服務異常，工程團隊已介入。最新進度請見狀態頁。」**

---

# 4) 伺服器異常時的降級頁面（Degraded / Fail-safe）

### 觸發條件

* 後端在 WS `hello/state` 回 `degraded_level ≥ 3`，或 REST 回 `503`/`degraded_mode`。
* 關鍵相依（Redis / DB / WS broker）超時率 ≥ 門檻。

### 降級行為

* **Level 1**：僅顯示提示，延長心跳，減少更新頻率。
* **Level 2**：限制單用戶出價頻率，暫停分佈圖即時更新（改 5s 批次）。
* **Level 3**：**只保留核心出價功能**（面板在、分佈/歷史/聊天室隱藏），顯示「系統繁忙，部分功能暫停」。
* **Level 4**：**排隊模式**：出價請求進佇列，顯示「排隊中…」；若超時（例 8 秒）未收到 `bid_accepted` 則回 `service_unavailable`＋重試建議。

### 降級頁（覆蓋層）文案

* 標題：**「系統繁忙，已啟用降級模式」**
* 說明：**「目前僅保留出價功能，其他即時資料暫停更新。您的出價將依到達順序處理。」**
* 行動：顯示**出價處理中**的 spinner、**可見的伺服器時間**；必要時提供「回到列表」。

### 簡易前端控制（依 degradedLevel 切換 UI）

```ts
function DegradedGate({ level, children }: { level: number; children: React.ReactNode }) {
  if (level <= 1) return <>{children}</>;
  if (level === 2) return <>{/* 隱藏分佈即時/降頻 */}{children}</>;
  if (level === 3) return (
    <>
      <Banner color="orange" text="系統繁忙，部分功能暫停（僅保留出價）。" />
      <Hide nonCore>{children}</Hide>
    </>
  );
  return (
    <FullOverlay>
      <h3>系統繁忙，已啟用降級模式</h3>
      <p>目前僅保留出價功能，其他即時資料暫停更新。</p>
      <ServerClock />
      <MinimalBidPanel />
    </FullOverlay>
  );
}
```

---

## 後端配合（小清單）

* **錯誤一致**：REST 與 WS 都用相同 `ErrorCode`；回應皆含 `server_time`。
* **冷卻秒數**：`too_frequent`/`rate_limited` 回傳 `cooldown_seconds`，前端顯示倒數。
* **區間提示**：`out_of_range` 可回 `allowed_min_bid/allowed_max_bid`（你的規則允許公開區間）。
* **degraded\_level**：在 `hello` 與每次 `state`、`extended` 事件都帶上，前端即可即時切換。
* **/status**：提供健康度與預計恢復時間（若已知）。
* **公告快取**：公告列表在 CDN/Redis 快取 30–60 秒即可（事故時可改短）。

---

## QA 測試劇本（建議自動化）

1. **網路斷線 10 秒**：banner 正確顯示、出價按鈕 disabled、重連後自動 `resume`、倒數與伺服器時間一致。
2. **出價錯誤**：逐一觸發所有錯誤碼，檢查文案、可修復行為、冷卻倒數。
3. **維護預告→維護中**：時間到自動切只讀；時間到解除。
4. **降級 3→4**：UI 漸進隱藏非核心，排隊模式可收到最終 `bid_accepted` 或合理超時。
5. **軟關閉期間斷線**：重連 `resume` 能收到 `extended` 事件；未收到 `bid_accepted` 不應顯示成功。

---

9. Phase 分工更細化

---

# Phase 1 — 核心拍賣功能（REST + DB + 後台操作）

## 範圍（Scope）

* 盲標拍賣的**全部商業規則**先用 REST 落地：建立/啟用/取消、出價提交、軟關閉延長、結束回寫名次、通知。
* **不含即時 WS 廣播**（延長/狀態改變以 REST 取回即可）。

## 分工

* Backend（BE）：DB schema + REST + Finalize Job + 通知佇列
* Frontend（FE）：列表/詳情頁、出價表單與錯誤處理
* Infra：Cloud Run（app / job）、Cloud SQL 建置、密鑰管理（JWT/DB/郵件）
* QA：API/E2E 測試劇本、自動化 smoke

## 交付物（Artifacts）

1. **資料庫**

   * `migrations/0001_auction_core.up.sql`、`0002_seed_statuses.up.sql`（你已有）
   * 初始索引與檢查約束（≤61 天、價格區間 check）
2. **REST API**

   * `POST /api/v1/auctions`（建立）
   * `POST /api/v1/auctions/{id}:activate`（啟用）
   * `POST /api/v1/auctions/{id}:cancel`（取消）
   * `GET  /api/v1/auctions`（列表，支援狀態/排序/分頁）
   * `GET  /api/v1/auctions/{id}`（詳情，含 viewer.can\_bid/alias）
   * `POST /api/v1/auctions/{id}/bids`（**盲標出價**，含區間驗證、黑名單、頻率限制）
   * `GET  /api/v1/auctions/{id}/my-bids`（僅看自己的出價）
   * `GET  /api/v1/auctions/{id}/results`（**ended** 後回前 7 名—匿名）
3. **批次作業**

   * Finalize Job（Cloud Run Job + Scheduler）：到期或強制結束 → 排序寫 `final_rank`、更新 `status=ended`、寫事件、排通知
4. **通知**

   * 通知封裝：winner / top7 / participant\_end / seller\_result（email/LINE 任選一種先通）
   * 模板檔與寄送重試（至多 3 次）
5. **後台與運維**

   * 黑名單管理 API：`GET/POST/DELETE /admin/blacklist`
   * 簡易審計查詢（讀 `audit_logs`）
6. **前端**

   * 拍賣列表 + 詳頁（盲標：看不到他人出價）
   * 出價表單：**明確錯誤文案**（區間外/過於頻繁/已截止/黑名單…）
7. **文件**

   * OpenAPI（REST 完整版）
   * Runbook（Finalize Job 失敗處置、人工結束流程）
   * 「拍賣規則」對外 Markdown

## 驗收標準（Acceptance / Exit Criteria）

* 功能驗收

  * ✅ 建立→啟用→（延長≥1 次）→結束→排名→發通知，全鍊路通
  * ✅ 盲標：**active/extended 狀態下，任何 API 都不回他人出價明細**
  * ✅ 區間驗證：`amount ∈ [min,max]`，否則 `409 out_of_range`
  * ✅ 軟關閉：**剩 ≤180s** 內有有效出價 → `end_at` 延長 60s（可重複），REST 重新讀取可見新截止
  * ✅ 黑名單：黑名單使用者出價 → `403 blacklisted`
  * ✅ 61 天限制：超過 → `422 duration_exceeded`
* 正確性 / 一致性

  * ✅ 結束時計名：同額出價時以 `created_at ASC` 排序；回寫 `final_rank` 正確
  * ✅ 冪等：相同 `(auction_id,bidder_id,client_seq)` 重送不新增
  * ✅ 審計：重要操作（activate/cancel/extend/finalize/bid）都有事件與 audit log
* 測試覆蓋

  * ✅ 單元測試：核心規則（區間、軟關閉、排名）**≥80%**
  * ✅ E2E：10 條關鍵劇本（正常投標、超時、延長、黑名單、相同金額競合…）**全通**
* 非功能

  * ✅ 出價 API P95 < **350ms**（單實例、無 WS）
  * ✅ 最後 10 分鐘內可承受 ≥ **20 req/s** 出價（單區域）無資料錯亂
  * ✅ 錯誤碼/訊息與對外文案一致

---

# Phase 2 — WebSocket 即時功能（廣播 / 斷線恢復 / 降級）

## 範圍（Scope）

* 以既定協定型別上線 WS：`hello/state/extended/bid_accepted/bid_rejected/closed/resume_ok/ping/pong`
* 仍為**密封投標**：不廣播金額/身份；只回當事人 `bid_*`

## 分工

* BE：WS Handler（Gin + Gorilla/WebSocket 或標準庫）、事件流（DB→Redis Pub/Sub）、斷線恢復
* FE：WS 客戶端 wrapper（指數退避、resume、狀態 banner、降級切換）
* Infra：Redis（Pub/Sub + 鎖）、水平擴展、多實例黏著/房間路由
* QA：網路混沌測試、長連線/重連測試

## 交付物（Artifacts）

1. **WS 服務**

   * `GET /ws/auctions/{id}?token=JWT&last_event_id=...`
   * 心跳 `ping/pong`（25s；可配置）
   * 斷線恢復：`resume(last_event_id)` → `resume_ok(missed=…)`
   * 廣播：`state/extended/closed`；私訊：`bid_accepted/bid_rejected`
2. **事件產生**

   * 出價成功 → 寫 `auction_events(bid_accepted)`（含 `event_id`）→ 私訊
   * 軟關閉 → `auction_events(extended)` → 廣播
   * 結束 → `auction_events(closed)` → 廣播
3. **降級控制**

   * 伺服器計算 `degraded_level (0–4)`，附加在 `hello/state/extended`
   * L3/L4 降級：僅保留 `place_bid` 與 `extended/closed` 推送（你規劃已列）
4. **FE 客戶端**

   * `useAuctionWS()` hook：重連策略（你提供的指數退避）、自動 resume、server clock 偏移校正
   * UI：**離線/重連**橫幅、**降級提示**、**出價結果 toast**
5. **觀測**

   * 指標：連線數、平均連線時長、resume 成功率、事件延遲（產生→送達 P95）

## 驗收標準（Acceptance / Exit Criteria）

* 功能驗收

  * ✅ 兩人同時在最後 180s 內出價 → **只延長一次**且全房間在**1s 內**收到 `extended`
  * ✅ 斷線 10s → 自動重連並 `resume`，最多補齊 500 則事件
  * ✅ 當事人能立即收到 `bid_accepted/bid_rejected`，其他人**絕不**收到金額/身份
* 正確性 / 一致性

  * ✅ 多實例下事件**不重複**且**不遺漏**（以 `event_id` 去重）
  * ✅ `degraded_level` 改變時，FE 能切換 UI（驗證 L1→L4）
* 非功能

  * ✅ `extended` 廣播 P95 < **800ms**（事件產生→所有連線收到）
  * ✅ 單拍賣房間穩定承載 **1,000+** 同連（讀多寫少）
  * ✅ Ping 超時能自動降級與重連，恢復後 `server_time` 偏差 < **200ms**
* 測試

  * ✅ 混沌測試：隨機斷網/延遲/丟包，恢復率 **≥ 99%**
  * ✅ 壓測：最後 10 分鐘內 10 req/s 出價 + 1k 連線，延遲在門檻內

---

# Phase 3 — 風控與優化（降級、風險、監控、快取、公告）

## 範圍（Scope）

* 風險管控、異常偵測、併發與快取優化、公告/維護機制、降級頁面與排隊模式、可觀測性與 SLO

## 分工

* BE：風控策略/降級排隊/公告 API、快取（L1/L2）、排程與熔斷
* FE：公告/維護/降級頁、錯誤體驗、分佈圖遞延更新
* Infra：監控告警、WAF/Rate limit、容量與成本觀測
* Data/QA：分佈圖後台任務、異常出價規則測試

## 交付物（Artifacts）

1. **風控**

   * 頻率限制：同拍賣同用戶 **5s/次**；全局出價 **12 req/min/user**
   * 黑名單執行；管理端查詢/解除
   * 異常規則（v1）：同帳號/裝置/IP 群聚、極端增幅、短時重複 → 標記 `bids.accepted=false, reject_reason=...` 或人工審核佇列
2. **降級與排隊**

   * Degraded Level 決策器（CPU / Redis 連線 / DB 延遲 / 錯誤率）
   * L4 出價排隊：Queue + TTL（8s），成功出價回 `bid_accepted`，超時回 `service_unavailable` + 建議重試
3. **快取與效能**

   * L1 應用快取（30s）：拍賣基本資訊、參與統計
   * L2 Redis（5m）：詳情、最近 50 筆**個人**出價、分佈快照
   * 「最後 10 分鐘」策略：停用 L1、提高 WS 頻率、優先處理該拍賣
4. **公告與狀態**

   * `GET /api/v1/announcements?active=true`（level：info/maintenance/incident）
   * `GET /api/v1/status`（`healthy`、`degraded_level`、`now`）
   * 後台公告 CRUD；FE 全站 banner/只讀模式切換
5. **分佈圖（UI 用）**

   * 背景任務每 5 分鐘寫 `auction_bid_histograms`；只輸出 **k≥5** 的桶
6. **可觀測性**

   * 指標：出價成功率、拒絕率（類型細分）、延長次數分布、WS 恢復率、降級時長、通知成功率
   * 告警：Finalize Job 卡住、Redis 連線錯誤率、DB P95>500ms、`service_unavailable` 比例升高
7. **安全**

   * WAF/Rate limit、JWT 失效/撤銷、CSRF（如走 Cookie）、參數化 SQL、審計查詢頁

## 驗收標準（Acceptance / Exit Criteria）

* 功能與體驗

  * ✅ 維護預告→維護中→恢復的公告流程可驗；維護中**只讀**（出價 disabled）
  * ✅ 降級 L1→L4 UI & 行為正確；L4 排隊模式可成功/超時有明確回覆
  * ✅ 分佈圖每 5 分鐘更新；最後 10 分鐘停用 L1 快取策略生效
* 風控效果

  * ✅ `too_frequent` 正常工作；黑名單阻擋率 100%
  * ✅ 異常規則能擋下你設計的測試樣本（自標/拉高價/連環投）
* 非功能 / SLO

  * ✅ 出價 API 可用性 **≥ 99.9%**（月）
  * ✅ `extended` 廣播在降級 L3 仍 P95 < **1200ms**
  * ✅ 通知送達成功率 **≥ 98%**（重試後）
* 觀測與告警

  * ✅ 主要指標在監控板可見；告警閾值可觸發並收到通知
* 安全

  * ✅ OWASP Top 10 基本測試通過；敏感操作均有 audit log

---

## 小總結：每階段的「驗收門檻」

* **Phase 1 出場門檻**：沒有 WS 的情況下，**靠 REST 就能無痛完成一場拍賣**（含延長與結束通知），E2E 全綠。
* **Phase 2 出場門檻**：**即時體驗穩**（延長 1s 內到、斷線 10s 內恢復且補齊事件）、多實例不丟事件。
* **Phase 3 出場門檻**：**逆風也可出價**（降級/排隊不散單）、事故/維護時**可預期**、風控可擋明顯異常、SLO 達標。

---



10. 監控與日誌
- 拍賣關鍵指標監控 (出價數、活躍用戶數)
- 系統性能監控 (WebSocket 連接數、響應時間)
- 業務異常告警 (大量異常出價、系統錯誤)
- 操作審計日誌 (管理員操作、重要業務操作)