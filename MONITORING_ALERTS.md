# 📊 Business Exchange Marketplace - 監控指標與警報規則

## 📋 目錄

- [監控架構概覽](#監控架構概覽)
- [系統指標](#系統指標)
- [應用指標](#應用指標)
- [業務指標](#業務指標)
- [警報規則](#警報規則)
- [儀表板配置](#儀表板配置)
- [SRE 指標](#sre-指標)
- [壓測監控](#壓測監控)

---

## 🏗️ 監控架構概覽

### 技術棧
- **雲端監控**: Google Cloud Monitoring (Stackdriver)
- **日誌聚合**: Google Cloud Logging
- **APM 追蹤**: Google Cloud Trace
- **自定義指標**: Prometheus + Grafana
- **實時警報**: PagerDuty + Slack + Email
- **性能分析**: Google Cloud Profiler

### 監控層次

```
┌─────────────────────────────────────────────┐
│                業務指標                      │
│  用戶註冊率 | 拍賣成交率 | 收入指標         │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│                應用指標                      │
│  API 延遲 | 錯誤率 | WebSocket 連接數      │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│                系統指標                      │
│  CPU | 記憶體 | 磁碟 | 網路 | 容器健康     │
└─────────────────────────────────────────────┘
┌─────────────────────────────────────────────┐
│              基礎設施指標                    │
│  Cloud Run | Cloud SQL | Redis | 負載均衡   │
└─────────────────────────────────────────────┘
```

---

## 💻 系統指標

### Cloud Run 服務指標

#### CPU 使用率
```yaml
metric_name: "run.googleapis.com/container/cpu/utilizations"
description: "Cloud Run 容器 CPU 使用率百分比"
unit: "percent"
thresholds:
  warning: 70%
  critical: 85%
  emergency: 95%
query_example: |
  fetch cloud_run_revision
  | metric 'run.googleapis.com/container/cpu/utilizations'
  | group_by 1m, [mean]
```

#### 記憶體使用率
```yaml
metric_name: "run.googleapis.com/container/memory/utilizations"
description: "Cloud Run 容器記憶體使用率百分比"
unit: "percent"
thresholds:
  warning: 80%
  critical: 90%
  emergency: 95%
query_example: |
  fetch cloud_run_revision
  | metric 'run.googleapis.com/container/memory/utilizations'
  | group_by 1m, [mean]
```

#### 容器實例數
```yaml
metric_name: "run.googleapis.com/container/instance_count" 
description: "活躍的 Cloud Run 容器實例數量"
unit: "count"
thresholds:
  max_instances: 10
  scale_up_threshold: 8
  scale_down_threshold: 2
query_example: |
  fetch cloud_run_revision
  | metric 'run.googleapis.com/container/instance_count'
  | group_by 1m, [mean]
```

### 資料庫指標 (Cloud SQL)

#### 連接數
```yaml
metric_name: "cloudsql.googleapis.com/database/mysql/connections"
description: "資料庫活躍連接數"
unit: "count"
thresholds:
  warning: 80    # 80 connections
  critical: 95   # 95 connections  
  max_allowed: 100
query_example: |
  fetch cloudsql_database
  | metric 'cloudsql.googleapis.com/database/mysql/connections'
  | group_by 1m, [mean]
```

#### CPU 使用率
```yaml
metric_name: "cloudsql.googleapis.com/database/cpu/utilization"
description: "Cloud SQL CPU 使用率"
unit: "percent"
thresholds:
  warning: 70%
  critical: 85%
  emergency: 95%
```

#### 查詢執行時間
```yaml
metric_name: "cloudsql.googleapis.com/database/mysql/queries"
description: "MySQL 查詢執行時間統計"
unit: "seconds"
thresholds:
  slow_query_threshold: 2s
  very_slow_threshold: 10s
```

### Redis 指標 (Cloud Memorystore)

#### 記憶體使用率
```yaml
metric_name: "redis.googleapis.com/stats/memory/usage_ratio"
description: "Redis 記憶體使用率"
unit: "percent"
thresholds:
  warning: 75%
  critical: 85%
  emergency: 90%
```

#### 連接數
```yaml
metric_name: "redis.googleapis.com/stats/connections/total"
description: "Redis 總連接數"
unit: "count" 
thresholds:
  warning: 80
  critical: 95
  max_allowed: 100
```

#### 緩存命中率
```yaml
metric_name: "redis.googleapis.com/stats/cache_hit_ratio"
description: "Redis 緩存命中率"
unit: "percent"
thresholds:
  warning: 90%   # 低於 90% 發出警告
  critical: 80%  # 低於 80% 嚴重警告
```

---

## 🖥️ 應用指標

### HTTP API 指標

#### 請求延遲 (P50, P95, P99)
```yaml
metric_name: "run.googleapis.com/request_latencies"
description: "HTTP 請求延遲分佈"
unit: "milliseconds"
percentiles:
  p50_threshold: 200ms
  p95_threshold: 500ms
  p99_threshold: 1000ms
labels:
  - service_name
  - endpoint
  - method
query_example: |
  fetch cloud_run_revision
  | metric 'run.googleapis.com/request_latencies'
  | group_by 1m, [percentile(95)]
  | filter resource.service_name == 'business-exchange-frontend'
```

#### 錯誤率
```yaml
metric_name: "custom/http_error_rate"
description: "HTTP 4xx/5xx 錯誤率"
unit: "percent"
thresholds:
  4xx_warning: 5%
  4xx_critical: 10%
  5xx_warning: 1%
  5xx_critical: 5%
calculation: |
  (sum(http_requests{code=~"4..|5.."}) / sum(http_requests)) * 100
```

#### 吞吐量 (RPS)
```yaml
metric_name: "run.googleapis.com/request_count"
description: "每秒請求數"
unit: "requests/second"
baseline: 
  normal: 10-100 RPS
  peak: 200-500 RPS
  max_expected: 1000 RPS
query_example: |
  fetch cloud_run_revision
  | metric 'run.googleapis.com/request_count'
  | group_by 1m, [rate]
```

### WebSocket 指標

#### 連接數
```yaml
metric_name: "custom/websocket_connections_total"
description: "WebSocket 活躍連接總數"
unit: "count"
thresholds:
  normal: 0-100
  busy: 100-500
  high_load: 500-1000
  max_capacity: 1000
labels:
  - auction_id
  - connection_state
```

#### 消息處理延遲
```yaml
metric_name: "custom/websocket_message_latency"
description: "WebSocket 消息處理延遲"
unit: "milliseconds"
thresholds:
  p95_target: 100ms
  p95_warning: 200ms
  p95_critical: 500ms
```

#### 連接建立失敗率
```yaml
metric_name: "custom/websocket_connection_failures"
description: "WebSocket 連接建立失敗率"
unit: "percent"
thresholds:
  warning: 2%
  critical: 5%
calculation: |
  (websocket_failed_connections / websocket_total_attempts) * 100
```

### 拍賣系統特定指標

#### 出價處理延遲
```yaml
metric_name: "custom/auction_bid_processing_time"
description: "拍賣出價處理時間"
unit: "milliseconds"
thresholds:
  p95_target: 50ms
  p95_warning: 100ms
  p95_critical: 200ms
labels:
  - auction_type
  - auction_status
```

#### 拍賣房間負載
```yaml
metric_name: "custom/auction_rooms_active"
description: "活躍拍賣房間數量"
unit: "count"
thresholds:
  normal: 0-10
  busy: 10-50
  high_load: 50-100
```

#### WebSocket Hub 統計
```yaml
metric_name: "custom/websocket_hub_stats"
description: "WebSocket Hub 連接統計"
metrics:
  - total_connections
  - auction_rooms_count
  - messages_per_second
  - connection_upgrades_failed
```

---

## 📈 業務指標

### 用戶行為指標

#### 用戶註冊率
```yaml
metric_name: "custom/user_registration_rate"
description: "每日新用戶註冊數"
unit: "count/day"
targets:
  daily_goal: 50
  weekly_goal: 300
  monthly_goal: 1200
labels:
  - registration_source
  - user_type
```

#### 用戶活躍度
```yaml
metric_name: "custom/user_activity"
description: "用戶活躍度指標"
metrics:
  - daily_active_users (DAU)
  - weekly_active_users (WAU) 
  - monthly_active_users (MAU)
  - session_duration_avg
targets:
  DAU_growth_rate: 5% monthly
  session_duration: 10 minutes avg
```

### 拍賣業務指標

#### 拍賣成交率
```yaml
metric_name: "custom/auction_completion_rate"
description: "拍賣成功完成率"
unit: "percent"
calculation: |
  (completed_auctions / total_auctions) * 100
targets:
  english_auctions: 85%
  sealed_auctions: 70%
  overall_target: 80%
```

#### 出價活躍度
```yaml
metric_name: "custom/bidding_activity"
description: "出價活動指標"
metrics:
  - bids_per_auction_avg
  - unique_bidders_per_auction
  - bid_frequency_per_user
  - peak_concurrent_bidders
targets:
  bids_per_auction: 15 (avg)
  participation_rate: 60%
```

#### 收入指標
```yaml
metric_name: "custom/revenue_metrics"
description: "收入相關指標"
metrics:
  - total_transaction_volume
  - average_transaction_value
  - commission_revenue
  - revenue_per_user (ARPU)
targets:
  monthly_revenue_growth: 10%
  ARPU_target: 500 TWD
```

### 系統可用性指標

#### 服務可用性 (SLA)
```yaml
sla_targets:
  overall_availability: 99.9%    # 8.76 小時/年停機
  api_availability: 99.95%       # 4.38 小時/年停機
  websocket_availability: 99.5%  # 43.8 小時/年停機
  
calculation: |
  uptime = (total_time - downtime) / total_time * 100
  
measurement_period: "rolling_30_days"
```

---

## 🚨 警報規則

### 緊急警報 (P0 - 立即響應)

#### 服務完全中斷
```yaml
alert_name: "ServiceCompleteOutage"
condition: |
  sum(rate(http_requests_total[5m])) == 0
  OR
  sum(up{job="backend"}) == 0
severity: "critical"
notification_channels:
  - pagerduty_critical
  - slack_emergency
  - sms_oncall
  - phone_oncall
response_time: 5 minutes
```

#### 資料庫連接失敗
```yaml
alert_name: "DatabaseConnectionFailure"
condition: |
  cloudsql_instance_up == 0
  OR
  increase(mysql_global_status_connection_errors_total[5m]) > 10
severity: "critical"
notification_channels:
  - pagerduty_critical
  - slack_emergency
  - email_dba_team
response_time: 5 minutes
```

#### 高錯誤率
```yaml
alert_name: "HighErrorRate"
condition: |
  (
    sum(rate(http_requests_total{code=~"5.."}[5m])) /
    sum(rate(http_requests_total[5m]))
  ) > 0.05
duration: "2m"
severity: "critical"
notification_channels:
  - pagerduty_high
  - slack_alerts
response_time: 10 minutes
```

### 警告警報 (P1 - 1小時內響應)

#### CPU 使用率過高
```yaml
alert_name: "HighCPUUsage"
condition: |
  avg(cpu_usage_percent) > 80
duration: "5m"
severity: "warning"
notification_channels:
  - slack_warnings
  - email_devops
response_time: 30 minutes
```

#### 記憶體使用率過高
```yaml
alert_name: "HighMemoryUsage" 
condition: |
  avg(memory_usage_percent) > 85
duration: "10m"
severity: "warning"
notification_channels:
  - slack_warnings
  - email_devops
response_time: 30 minutes
```

#### WebSocket 連接異常
```yaml
alert_name: "WebSocketConnectionIssues"
condition: |
  websocket_connection_failures_rate > 0.05
  OR
  avg(websocket_connections_total) > 800
duration: "3m"
severity: "warning"
notification_channels:
  - slack_auction_team
  - email_backend_team
response_time: 15 minutes
```

### 信息警報 (P2 - 4小時內響應)

#### 緩存命中率低
```yaml
alert_name: "LowCacheHitRate"
condition: |
  avg(redis_cache_hit_ratio) < 0.85
duration: "15m"
severity: "info"
notification_channels:
  - slack_performance
response_time: 2 hours
```

#### 異常用戶活動
```yaml
alert_name: "UnusualUserActivity"
condition: |
  abs(increase(user_registrations_total[1h]) - increase(user_registrations_total[1h] offset 1w)) > 100
  OR
  increase(failed_login_attempts_total[1h]) > 500
duration: "30m"
severity: "info"
notification_channels:
  - slack_security
  - email_product_team
response_time: 4 hours
```

### 業務警報

#### 拍賣成交率下降
```yaml
alert_name: "AuctionCompletionRateDown"
condition: |
  (
    sum(increase(auctions_completed_total[24h])) /
    sum(increase(auctions_total[24h]))
  ) < 0.75
duration: "2h"
severity: "warning"
notification_channels:
  - slack_business
  - email_product_manager
response_time: 4 hours
```

#### 收入指標異常
```yaml
alert_name: "RevenueMetricsAnomaly"
condition: |
  abs(sum(increase(transaction_revenue_total[1d])) - 
      sum(increase(transaction_revenue_total[1d] offset 1w))) > 50000
duration: "1d"
severity: "info"
notification_channels:
  - slack_finance
  - email_management
response_time: 24 hours
```

---

## 📊 儀表板配置

### 系統概覽儀表板

#### 服務健康概覽
```json
{
  "dashboard": "system_overview",
  "panels": [
    {
      "title": "服務狀態",
      "type": "stat",
      "targets": [
        {
          "expr": "up{job=\"frontend\"}",
          "legendFormat": "前端服務"
        },
        {
          "expr": "up{job=\"backend\"}",
          "legendFormat": "後端服務"
        },
        {
          "expr": "up{job=\"auction\"}",
          "legendFormat": "拍賣服務"
        }
      ],
      "thresholds": [
        {"color": "red", "value": 0},
        {"color": "green", "value": 1}
      ]
    },
    {
      "title": "請求延遲 (P95)",
      "type": "graph",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service))",
          "legendFormat": "{{service}} P95"
        }
      ],
      "yAxis": {"unit": "seconds"}
    }
  ]
}
```

### 應用效能儀表板

#### API 效能監控
```json
{
  "dashboard": "api_performance",
  "panels": [
    {
      "title": "API 請求量",
      "type": "graph",
      "targets": [
        {
          "expr": "sum(rate(http_requests_total[1m])) by (method, endpoint)",
          "legendFormat": "{{method}} {{endpoint}}"
        }
      ]
    },
    {
      "title": "錯誤率趨勢", 
      "type": "graph",
      "targets": [
        {
          "expr": "sum(rate(http_requests_total{code=~\"4..\"}[5m])) by (service)",
          "legendFormat": "4xx {{service}}"
        },
        {
          "expr": "sum(rate(http_requests_total{code=~\"5..\"}[5m])) by (service)",
          "legendFormat": "5xx {{service}}"
        }
      ]
    }
  ]
}
```

### WebSocket 監控儀表板

#### 拍賣系統即時監控
```json
{
  "dashboard": "websocket_auction_monitoring",
  "panels": [
    {
      "title": "WebSocket 連接數",
      "type": "stat",
      "targets": [
        {
          "expr": "websocket_connections_total",
          "legendFormat": "總連接數"
        }
      ]
    },
    {
      "title": "活躍拍賣房間",
      "type": "table",
      "targets": [
        {
          "expr": "auction_rooms_active",
          "format": "table"
        }
      ],
      "columns": ["拍賣ID", "連接數", "狀態", "最後活動"]
    },
    {
      "title": "消息處理延遲",
      "type": "heatmap",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, websocket_message_duration_seconds_bucket)",
          "legendFormat": "處理延遲"
        }
      ]
    }
  ]
}
```

### 業務指標儀表板

#### 業務KPI總覽
```json
{
  "dashboard": "business_kpi",
  "panels": [
    {
      "title": "今日關鍵指標",
      "type": "stat",
      "targets": [
        {
          "expr": "increase(user_registrations_total[1d])",
          "legendFormat": "新用戶註冊"
        },
        {
          "expr": "increase(auctions_completed_total[1d])",
          "legendFormat": "完成拍賣"
        },
        {
          "expr": "sum(increase(transaction_revenue_total[1d]))",
          "legendFormat": "今日收入 (TWD)"
        }
      ]
    },
    {
      "title": "用戶活躍度趨勢",
      "type": "graph",
      "targets": [
        {
          "expr": "daily_active_users",
          "legendFormat": "日活躍用戶"
        },
        {
          "expr": "weekly_active_users / 7",
          "legendFormat": "週活躍用戶 (日均)"
        }
      ]
    }
  ]
}
```

---

## 🎯 SRE 指標

### 服務等級目標 (SLO)

#### API 可用性
```yaml
slo_name: "api_availability"
objective: "99.9%"
measurement_window: "30d"
error_budget: "0.1%"  # 約 43.2 分鐘/月

sli_definition: |
  (
    sum(rate(http_requests_total{code!~"5.."}[5m])) /
    sum(rate(http_requests_total[5m]))
  )

error_budget_burn_alerts:
  - name: "fast_burn"
    condition: "error_budget_consumption_rate > 14.4x"  # 2小時內消耗完
    notification: "page_immediately"
    
  - name: "slow_burn" 
    condition: "error_budget_consumption_rate > 6x"     # 5天內消耗完
    notification: "alert_within_1h"
```

#### 請求延遲
```yaml
slo_name: "request_latency"
objective: "95% of requests < 500ms"
measurement_window: "7d"

sli_definition: |
  histogram_quantile(0.95, 
    sum(rate(http_request_duration_seconds_bucket[5m])) by (le)
  ) < 0.5

error_budget_policy:
  - if error_budget < 10%: freeze non-critical deployments
  - if error_budget < 5%: freeze all deployments
  - if error_budget < 1%: emergency response mode
```

### 關鍵用戶旅程 SLI

#### 用戶註冊流程
```yaml
user_journey: "user_registration"
steps:
  1. "訪問註冊頁面": 
     - sli: "page_load_time < 2s"
     - target: "95%"
  2. "提交註冊表單":
     - sli: "form_submission_success_rate > 99%"  
     - target: "99%"
  3. "郵件驗證":
     - sli: "email_delivery_time < 30s"
     - target: "90%"

overall_slo: "90% of users complete registration within 5 minutes"
```

#### 拍賣參與流程
```yaml
user_journey: "auction_participation"
steps:
  1. "瀏覽拍賣列表":
     - sli: "auction_list_load_time < 3s"
     - target: "95%"
  2. "進入拍賣房間":
     - sli: "websocket_connection_success_rate > 95%"
     - target: "95%"
  3. "提交出價":
     - sli: "bid_processing_time < 100ms"
     - target: "95%"
  4. "接收即時更新":
     - sli: "message_delivery_time < 200ms"
     - target: "90%"

overall_slo: "95% of bids processed successfully within 100ms"
```

---

## 🧪 壓測監控

### 壓力測試監控配置

#### 負載測試指標
```yaml
load_test_metrics:
  - name: "concurrent_users"
    target: 1000
    ramp_up_time: "10m"
    test_duration: "30m"
    
  - name: "requests_per_second"
    baseline: 100
    target: 500
    max_expected: 1000
    
  - name: "response_time_p95"
    baseline: 200ms
    target: 500ms
    failure_threshold: 1000ms
```

#### WebSocket 壓力測試
```yaml
websocket_load_test:
  - name: "concurrent_connections"
    target: 500
    ramp_up_time: "5m"
    
  - name: "messages_per_second"
    target: 1000
    burst_capacity: 2000
    
  - name: "connection_setup_time"
    target: "< 100ms (P95)"
    failure_threshold: "1s"
```

#### 資料庫壓力測試
```yaml
database_load_test:
  - name: "connection_pool_stress"
    concurrent_connections: 90
    query_rate: 1000/s
    
  - name: "query_performance"
    target_latency: "< 50ms (P95)"
    failure_threshold: "1s"
    
  - name: "transaction_throughput"  
    target: 500/s
    consistency_check: true
```

### 壓測警報閾值

#### 性能降級警報
```yaml
performance_degradation_alerts:
  - name: "ResponseTimeIncreased"
    condition: |
      (
        avg(http_request_duration_seconds{quantile="0.95"}) >
        avg(http_request_duration_seconds{quantile="0.95"} offset 1h) * 1.5
      )
    during_load_test: true
    severity: "warning"
    
  - name: "ErrorRateSpike"
    condition: |
      rate(http_requests_total{code=~"5.."}[5m]) > 0.02
    during_load_test: true
    severity: "critical"
```

#### 資源耗盡警報
```yaml
resource_exhaustion_alerts:
  - name: "ConnectionPoolExhaustion"
    condition: |
      mysql_connections_in_use / mysql_max_connections > 0.9
    during_load_test: true
    severity: "critical"
    
  - name: "MemoryExhaustion"
    condition: |
      memory_usage_percent > 90
    during_load_test: true
    severity: "warning"
```

---

## 📈 監控最佳實務

### 指標收集策略

#### 高頻指標 (每秒)
```yaml
high_frequency_metrics:
  - http_requests_total
  - http_request_duration_seconds
  - websocket_connections_total
  - websocket_messages_total
  
collection_interval: 1s
retention_period: 7d
```

#### 中頻指標 (每分鐘)
```yaml
medium_frequency_metrics:
  - cpu_usage_percent
  - memory_usage_percent  
  - disk_usage_percent
  - network_bytes_total
  
collection_interval: 60s
retention_period: 30d
```

#### 低頻指標 (每小時)
```yaml
low_frequency_metrics:
  - daily_active_users
  - auction_completion_rate_daily
  - revenue_metrics_hourly
  
collection_interval: 3600s
retention_period: 365d
```

### 監控成本最佳化

#### 指標清理策略
```yaml
metric_cleanup_policy:
  high_cardinality_metrics:
    - max_series: 10000
    - auto_cleanup: true
    - retention: 3d
    
  business_metrics:
    - aggregation_level: daily
    - raw_data_retention: 90d
    - aggregated_retention: 2y
    
  debug_metrics:
    - enabled_in: ["development", "staging"]
    - disabled_in: ["production"]
```

### 告警疲勞防護

#### 告警抑制規則
```yaml
alert_suppression:
  - name: "maintenance_window"
    suppresses: ["HighCPUUsage", "HighMemoryUsage"]
    schedule: "02:00-04:00 UTC daily"
    
  - name: "known_issues"
    suppresses: ["WebSocketConnectionIssues"]
    condition: "incident_ticket_open == true"
    max_duration: "24h"
```

#### 告警分組與去重
```yaml
alert_grouping:
  - group_by: ["service", "severity"]
    group_wait: "30s"
    group_interval: "10m"
    repeat_interval: "1h"
    
  - group_by: ["alertname"]
    matchers: ["severity=critical"]
    group_wait: "0s"
    repeat_interval: "5m"
```

---

## 🔧 實作清單

### 監控設置任務
- [ ] **Google Cloud Monitoring 配置**
  - [ ] 自定義指標定義
  - [ ] 儀表板創建
  - [ ] 告警政策設置
  - [ ] 通知渠道配置

- [ ] **應用程式指標整合**  
  - [ ] Go 應用 Prometheus 指標
  - [ ] Next.js 性能指標
  - [ ] WebSocket 監控實作
  - [ ] 業務指標追蹤

- [ ] **日誌聚合**
  - [ ] 結構化日誌格式
  - [ ] 日誌搜尋索引
  - [ ] 錯誤日誌聚合
  - [ ] 審計日誌追蹤

### 警報系統任務
- [ ] **多渠道通知**
  - [ ] PagerDuty 整合
  - [ ] Slack 機器人設置  
  - [ ] 郵件告警配置
  - [ ] SMS 緊急通知

- [ ] **警報規則完善**
  - [ ] SLO 基礎警報
  - [ ] 業務指標警報
  - [ ] 安全事件警報
  - [ ] 預測性警報

### 可視化與分析
- [ ] **監控儀表板**
  - [ ] 系統概覽儀表板
  - [ ] 應用效能儀表板
  - [ ] 業務指標儀表板
  - [ ] 安全監控儀表板

- [ ] **報告與分析**
  - [ ] 週/月性能報告
  - [ ] SLO 達成報告
  - [ ] 容量規劃分析
  - [ ] 故障根因分析

---

**文檔版本**: 1.0  
**最後更新**: 2024-09-03  
**維護人員**: SRE Team + DevOps Team  
**審核狀態**: ✅ 已審核  
**下次審查**: 2024-12-03