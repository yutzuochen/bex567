# 🚩 Business Exchange Marketplace - Feature Flags 管理

## 📋 目錄

- [Feature Flags 概述](#feature-flags-概述)
- [實作架構](#實作架構)
- [功能標記列表](#功能標記列表)
- [配置管理](#配置管理)
- [部署策略](#部署策略)
- [監控與追蹤](#監控與追蹤)
- [最佳實務](#最佳實務)

---

## 🏁 Feature Flags 概述

Feature Flags（功能標記）允許我們在不部署新程式碼的情況下，動態控制功能的啟用/停用，支援：
- **金絲雀發布**: 漸進式功能推出
- **A/B 測試**: 功能變體比較測試
- **緊急開關**: 快速關閉有問題的功能
- **用戶分群**: 針對特定用戶群體啟用功能

### 標記分類

| 類型 | 用途 | 生命週期 | 風險等級 |
|------|------|----------|----------|
| **🔄 Release Flags** | 功能發布控制 | 短期 (1-4 週) | 低 |
| **🧪 Experiment Flags** | A/B 測試實驗 | 中期 (1-3 月) | 中 |
| **⚡ Kill Switches** | 緊急功能關閉 | 長期 (永久) | 高 |
| **🎯 Permission Flags** | 權限控制 | 長期 (永久) | 中 |

---

## 🏗️ 實作架構

### 技術實作

#### 後端 (Go) 實作
```go
// internal/featureflags/flags.go
package featureflags

import (
    "context"
    "encoding/json"
    "time"
    "github.com/go-redis/redis/v8"
)

type FeatureFlagService struct {
    redis  *redis.Client
    cache  map[string]bool
    ctx    context.Context
}

type Flag struct {
    Name        string    `json:"name"`
    Enabled     bool      `json:"enabled"`
    Description string    `json:"description"`
    Environment string    `json:"environment"`
    UserGroups  []string  `json:"user_groups,omitempty"`
    Percentage  int       `json:"percentage,omitempty"` // 0-100
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

func NewFeatureFlagService(redisClient *redis.Client) *FeatureFlagService {
    return &FeatureFlagService{
        redis: redisClient,
        cache: make(map[string]bool),
        ctx:   context.Background(),
    }
}

func (f *FeatureFlagService) IsEnabled(flagName string, userID uint64) bool {
    // 1. 檢查快取
    if enabled, exists := f.cache[flagName]; exists {
        return enabled
    }
    
    // 2. 從 Redis 獲取
    flagData := f.redis.Get(f.ctx, "feature_flag:"+flagName).Val()
    if flagData == "" {
        return false
    }
    
    var flag Flag
    json.Unmarshal([]byte(flagData), &flag)
    
    // 3. 檢查用戶分群邏輯
    if len(flag.UserGroups) > 0 {
        return f.checkUserGroup(userID, flag.UserGroups)
    }
    
    // 4. 百分比推出
    if flag.Percentage > 0 {
        return f.checkPercentage(userID, flag.Percentage)
    }
    
    // 5. 更新本地快取
    f.cache[flagName] = flag.Enabled
    return flag.Enabled
}

func (f *FeatureFlagService) checkPercentage(userID uint64, percentage int) bool {
    // 基於用戶 ID 的一致性雜湊
    hash := userID % 100
    return int(hash) < percentage
}
```

#### 前端 (TypeScript) 實作
```typescript
// src/lib/featureFlags.ts
export interface FeatureFlag {
  name: string;
  enabled: boolean;
  description: string;
  environment: string;
  userGroups?: string[];
  percentage?: number;
}

export class FeatureFlagService {
  private cache: Map<string, boolean> = new Map();
  private apiUrl: string;

  constructor(apiUrl: string) {
    this.apiUrl = apiUrl;
  }

  async isEnabled(flagName: string, userID?: number): Promise<boolean> {
    // 檢查本地快取
    if (this.cache.has(flagName)) {
      return this.cache.get(flagName)!;
    }

    try {
      const response = await fetch(`${this.apiUrl}/api/v1/feature-flags/${flagName}`, {
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          ...(userID && { 'X-User-ID': userID.toString() })
        }
      });

      const data = await response.json();
      const enabled = data.enabled || false;
      
      // 快取結果 (5 分鐘)
      this.cache.set(flagName, enabled);
      setTimeout(() => this.cache.delete(flagName), 5 * 60 * 1000);
      
      return enabled;
    } catch (error) {
      console.warn(`Feature flag ${flagName} check failed:`, error);
      return false; // 失敗時預設關閉
    }
  }

  // React Hook 使用
  useFeatureFlag(flagName: string): boolean {
    const [enabled, setEnabled] = useState(false);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
      this.isEnabled(flagName).then(result => {
        setEnabled(result);
        setLoading(false);
      });
    }, [flagName]);

    return loading ? false : enabled;
  }
}

export const featureFlags = new FeatureFlagService(
  process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'
);
```

---

## 🎯 功能標記列表

### 🔄 Release Flags (發布控制)

#### RF-001: 新用戶註冊流程
```json
{
  "name": "new_user_registration_flow",
  "enabled": false,
  "description": "啟用新的用戶註冊流程，包含身份驗證和 KYC",
  "environment": "all",
  "type": "release",
  "rollout_strategy": "percentage",
  "target_percentage": 0,
  "created_by": "product-team",
  "jira_ticket": "BEM-123",
  "expected_removal": "2024-10-15"
}
```

#### RF-002: 英式拍賣系統
```json
{
  "name": "english_auction_system",
  "enabled": true,
  "description": "啟用英式拍賣功能，支援即時競標和自動延長",
  "environment": "all",
  "type": "release",
  "rollout_strategy": "user_groups",
  "target_groups": ["beta_users", "power_users"],
  "created_by": "auction-team",
  "jira_ticket": "BEM-456",
  "expected_removal": "2024-09-30"
}
```

#### RF-003: 新版商業清單頁面
```json
{
  "name": "new_listing_page_ui",
  "enabled": false,
  "description": "新版商業清單頁面 UI，包含進階搜尋和篩選功能",
  "environment": "staging,production",
  "type": "release",
  "rollout_strategy": "percentage",
  "target_percentage": 10,
  "created_by": "frontend-team",
  "jira_ticket": "BEM-789",
  "expected_removal": "2024-11-01"
}
```

### 🧪 Experiment Flags (實驗性功能)

#### EF-001: A/B 測試 - 推薦演算法
```json
{
  "name": "recommendation_algorithm_v2",
  "enabled": true,
  "description": "測試新的機器學習推薦演算法效果",
  "environment": "production",
  "type": "experiment",
  "variants": {
    "control": 50,
    "ml_based": 50
  },
  "success_metrics": ["click_through_rate", "conversion_rate"],
  "test_duration": "30 days",
  "created_by": "data-science-team",
  "jira_ticket": "BEM-201"
}
```

#### EF-002: 付款方式實驗
```json
{
  "name": "payment_methods_experiment",
  "enabled": true,
  "description": "測試顯示更多付款方式對轉換率的影響",
  "environment": "production",
  "type": "experiment",
  "variants": {
    "standard": 40,
    "extended": 60
  },
  "success_metrics": ["payment_completion_rate", "cart_abandonment_rate"],
  "test_duration": "21 days",
  "created_by": "payments-team",
  "jira_ticket": "BEM-202"
}
```

### ⚡ Kill Switches (緊急開關)

#### KS-001: WebSocket 即時更新
```json
{
  "name": "websocket_realtime_updates",
  "enabled": true,
  "description": "拍賣系統 WebSocket 即時更新功能緊急開關",
  "environment": "all",
  "type": "kill_switch",
  "priority": "high",
  "fallback_behavior": "polling_every_5s",
  "alert_on_disable": true,
  "created_by": "auction-team",
  "documentation": "如果 WebSocket 服務不穩定，可關閉此標記回退到輪詢模式"
}
```

#### KS-002: 自動出價系統
```json
{
  "name": "auto_bidding_system",
  "enabled": true,
  "description": "自動出價系統緊急開關",
  "environment": "all",
  "type": "kill_switch",
  "priority": "critical",
  "fallback_behavior": "manual_bidding_only",
  "alert_on_disable": true,
  "created_by": "auction-team",
  "documentation": "如發現自動出價邏輯異常，立即關閉此標記"
}
```

#### KS-003: 第三方整合服務
```json
{
  "name": "third_party_integrations",
  "enabled": true,
  "description": "所有第三方服務整合的總開關",
  "environment": "all",
  "type": "kill_switch",
  "priority": "medium",
  "services": ["payment_gateway", "sms_service", "email_service"],
  "fallback_behavior": "local_fallback",
  "alert_on_disable": true,
  "created_by": "platform-team"
}
```

### 🎯 Permission Flags (權限控制)

#### PF-001: 管理員功能
```json
{
  "name": "admin_panel_access",
  "enabled": true,
  "description": "管理員控制面板訪問權限",
  "environment": "all",
  "type": "permission",
  "user_groups": ["admin", "super_admin"],
  "features": ["user_management", "system_settings", "audit_logs"],
  "created_by": "security-team"
}
```

#### PF-002: Beta 功能訪問
```json
{
  "name": "beta_features_access",
  "enabled": true,
  "description": "Beta 功能訪問權限",
  "environment": "all",
  "type": "permission",
  "user_groups": ["beta_users", "internal_users"],
  "features": ["advanced_search", "analytics_dashboard"],
  "created_by": "product-team"
}
```

---

## ⚙️ 配置管理

### 環境配置

#### 開發環境 (Development)
```yaml
# config/feature-flags-dev.yaml
feature_flags:
  default_enabled: true  # 開發環境預設啟用所有功能
  cache_ttl: 60          # 快取時間 60 秒
  
  flags:
    new_user_registration_flow:
      enabled: true
      percentage: 100
    
    english_auction_system:
      enabled: true
      user_groups: ["all"]
      
    websocket_realtime_updates:
      enabled: true
```

#### 測試環境 (Staging)  
```yaml
# config/feature-flags-staging.yaml
feature_flags:
  default_enabled: false # 測試環境謹慎啟用
  cache_ttl: 300         # 快取時間 5 分鐘
  
  flags:
    new_user_registration_flow:
      enabled: true
      percentage: 50       # 50% 用戶測試
      
    english_auction_system:
      enabled: true
      user_groups: ["internal_users", "beta_users"]
```

#### 生產環境 (Production)
```yaml
# config/feature-flags-prod.yaml
feature_flags:
  default_enabled: false # 生產環境保守啟用
  cache_ttl: 600         # 快取時間 10 分鐘
  
  flags:
    english_auction_system:
      enabled: true
      percentage: 100      # 已穩定，全量啟用
      
    new_user_registration_flow:
      enabled: false       # 待進一步測試
      percentage: 5        # 僅 5% 用戶
```

### Redis 存儲結構
```redis
# 標記基本資訊
SET feature_flag:english_auction_system '{"name":"english_auction_system","enabled":true,"percentage":100}'

# 用戶分群映射
SADD user_group:beta_users 123 456 789
SADD user_group:power_users 456 789 1011

# 實驗分組
HSET experiment:recommendation_algorithm_v2:assignments user:123 "control"
HSET experiment:recommendation_algorithm_v2:assignments user:456 "ml_based"

# 標記變更歷史
ZADD feature_flag_history:english_auction_system 1693747200 "enabled:true:user:admin"
```

---

## 🚀 部署策略

### 金絲雀發布 (Canary Deployment)

#### 階段 1: 內部測試 (1-2 天)
```json
{
  "name": "new_feature_canary_phase1",
  "enabled": true,
  "user_groups": ["internal_users", "qa_team"],
  "percentage": 0,
  "monitoring": {
    "error_threshold": "1%",
    "latency_threshold": "200ms",
    "success_criteria": "zero_critical_bugs"
  }
}
```

#### 階段 2: Beta 用戶 (3-5 天)
```json
{
  "name": "new_feature_canary_phase2", 
  "enabled": true,
  "user_groups": ["beta_users"],
  "percentage": 10,
  "monitoring": {
    "error_threshold": "0.5%",
    "latency_threshold": "300ms", 
    "success_criteria": "positive_user_feedback"
  }
}
```

#### 階段 3: 漸進式推出 (1-2 週)
```json
{
  "name": "new_feature_canary_phase3",
  "enabled": true,
  "percentage": 25,  // 每天增加 25%
  "rollout_schedule": {
    "day_1": 25,
    "day_2": 50,
    "day_3": 75,
    "day_4": 100
  }
}
```

### A/B 測試策略

#### 標準 A/B 測試配置
```typescript
interface ABTestConfig {
  name: string;
  variants: {
    [key: string]: {
      percentage: number;
      config: any;
    };
  };
  assignment_strategy: 'random' | 'user_id_hash';
  success_metrics: string[];
  minimum_sample_size: number;
  test_duration_days: number;
  significance_level: number; // 0.05
}

const abTestExample: ABTestConfig = {
  name: "checkout_flow_optimization",
  variants: {
    "control": {
      percentage: 50,
      config: { "steps": 3, "design": "current" }
    },
    "treatment": {
      percentage: 50, 
      config: { "steps": 2, "design": "simplified" }
    }
  },
  assignment_strategy: "user_id_hash",
  success_metrics: ["conversion_rate", "completion_time"],
  minimum_sample_size: 1000,
  test_duration_days: 14,
  significance_level: 0.05
};
```

---

## 📊 監控與追蹤

### 關鍵指標

#### 技術指標
- **標記評估延遲**: < 10ms (P95)
- **快取命中率**: > 95%
- **Redis 連接健康**: > 99.9%
- **配置同步延遲**: < 30s

#### 業務指標  
- **功能採用率**: 按標記統計用戶使用率
- **A/B 測試轉換率**: 實驗組 vs 控制組
- **錯誤率影響**: 新功能對整體錯誤率影響
- **用戶體驗指標**: 頁面載入時間、互動延遲

### 監控告警配置

#### CloudWatch/Stackdriver 指標
```yaml
# monitoring/feature-flags-alerts.yaml
alerts:
  - name: "FeatureFlag-HighLatency"
    condition: "feature_flag_evaluation_latency_p95 > 50ms"
    duration: "5m"
    severity: "warning"
    
  - name: "FeatureFlag-LowCacheHitRate"
    condition: "feature_flag_cache_hit_rate < 0.90"
    duration: "10m"
    severity: "warning"
    
  - name: "FeatureFlag-RedisConnectionFailure" 
    condition: "redis_connection_failures > 0"
    duration: "1m"
    severity: "critical"
    
  - name: "ABTest-SignificantDifference"
    condition: "ab_test_significance_level > 0.05"
    duration: "1h"
    severity: "info"
```

#### 自定義監控儀表板
```json
{
  "dashboard_name": "Feature Flags Overview",
  "widgets": [
    {
      "title": "標記評估 QPS",
      "type": "line_chart",
      "metric": "feature_flag_evaluations_per_second"
    },
    {
      "title": "啟用標記數量",
      "type": "stat",
      "metric": "enabled_feature_flags_count"
    },
    {
      "title": "A/B 測試活躍數",
      "type": "stat", 
      "metric": "active_ab_tests_count"
    },
    {
      "title": "標記配置變更",
      "type": "table",
      "data_source": "feature_flag_audit_log"
    }
  ]
}
```

### 審計與合規

#### 變更日誌記錄
```sql
-- feature_flag_audit_log 表結構
CREATE TABLE feature_flag_audit_log (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    flag_name VARCHAR(255) NOT NULL,
    action ENUM('created', 'enabled', 'disabled', 'updated', 'deleted') NOT NULL,
    old_value JSON,
    new_value JSON,
    changed_by VARCHAR(255) NOT NULL,
    change_reason TEXT,
    client_ip VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_flag_name (flag_name),
    INDEX idx_created_at (created_at),
    INDEX idx_changed_by (changed_by)
);
```

#### 自動化報告
```bash
#!/bin/bash
# scripts/generate-feature-flag-report.sh

REPORT_DATE=$(date +%Y-%m-%d)
OUTPUT_FILE="feature-flag-report-${REPORT_DATE}.json"

echo "📊 Generating Feature Flag Report for ${REPORT_DATE}"

# 獲取所有標記狀態
curl -s "http://localhost:8080/api/v1/admin/feature-flags/summary" | jq '.' > ${OUTPUT_FILE}

# 生成 Markdown 報告
cat > "feature-flag-report-${REPORT_DATE}.md" << EOF
# Feature Flag Report - ${REPORT_DATE}

## Active Flags
$(cat ${OUTPUT_FILE} | jq -r '.active_flags[] | "- **\(.name)**: \(.description)"')

## Recently Changed
$(cat ${OUTPUT_FILE} | jq -r '.recent_changes[] | "- \(.flag_name): \(.action) by \(.changed_by)"')

## A/B Tests Status  
$(cat ${OUTPUT_FILE} | jq -r '.ab_tests[] | "- **\(.name)**: \(.status) (\(.completion_percentage)%)"')
EOF

echo "✅ Report generated: feature-flag-report-${REPORT_DATE}.md"
```

---

## 📋 最佳實務

### 開發指導原則

#### 1. 標記命名規範
```
格式: [type]_[feature]_[description]
類型: rf (release), ef (experiment), ks (kill_switch), pf (permission)

✅ 好的命名:
- rf_user_registration_flow
- ef_recommendation_algorithm_v2  
- ks_websocket_realtime_updates
- pf_admin_panel_access

❌ 不好的命名:
- new_feature
- test_flag
- enable_xyz
```

#### 2. 標記生命週期管理
```typescript
// 標記清理計畫
const flagLifecycle = {
  // 短期標記 (1-4 週) - 功能發布後移除
  release_flags: {
    max_lifetime: "4 weeks",
    cleanup_strategy: "automatic_after_100_percent_rollout"
  },
  
  // 中期標記 (1-3 月) - 實驗結束後移除  
  experiment_flags: {
    max_lifetime: "3 months",
    cleanup_strategy: "manual_after_conclusion"
  },
  
  // 長期標記 - 定期審查
  kill_switches: {
    max_lifetime: "permanent", 
    review_frequency: "quarterly"
  }
};
```

#### 3. 程式碼整合模式
```typescript
// ❌ 不推薦 - 直接在組件中檢查標記
function UserProfile() {
  const newUIEnabled = useFeatureFlag('new_profile_ui');
  
  if (newUIEnabled) {
    return <NewUserProfile />;
  }
  return <OldUserProfile />;
}

// ✅ 推薦 - 使用組件包裝器
function UserProfile() {
  return (
    <FeatureFlag flag="new_profile_ui" fallback={<OldUserProfile />}>
      <NewUserProfile />
    </FeatureFlag>
  );
}

// ✅ 更好 - 使用配置驅動
const profileConfig = useFeatureFlagConfig('user_profile_settings');
function UserProfile() {
  return <UserProfile config={profileConfig} />;
}
```

### 安全考量

#### 1. 敏感標記保護
```json
{
  "name": "payment_processing_v2",
  "enabled": false,
  "security_level": "high",
  "require_approval": true,
  "approved_by": ["security-team", "product-owner"],
  "change_window": "maintenance_hours_only",
  "rollback_plan": "immediate_disable_on_error"
}
```

#### 2. 權限控制
```yaml
# rbac-feature-flags.yaml
roles:
  - name: "feature-flag-admin"
    permissions:
      - "feature_flags:*:*"
      
  - name: "product-manager"
    permissions:
      - "feature_flags:read:*"
      - "feature_flags:update:experiment_flags"
      
  - name: "developer"
    permissions:
      - "feature_flags:read:*"
      - "feature_flags:update:release_flags"
```

### 效能最佳化

#### 1. 快取策略
```go
// 多層快取架構
type CacheLayer struct {
    L1 *sync.Map        // 程序內記憶體快取
    L2 *redis.Client    // Redis 分散式快取  
    L3 *database.DB     // 資料庫持久層
}

func (c *CacheLayer) GetFlag(name string) (*Flag, error) {
    // L1: 檢查程序內快取 (< 1ms)
    if flag, ok := c.L1.Load(name); ok {
        return flag.(*Flag), nil
    }
    
    // L2: 檢查 Redis 快取 (< 10ms)
    if flagData, err := c.L2.Get(ctx, "flag:"+name).Result(); err == nil {
        flag := &Flag{}
        json.Unmarshal([]byte(flagData), flag)
        c.L1.Store(name, flag) // 寫入 L1
        return flag, nil
    }
    
    // L3: 從資料庫查詢 (< 50ms)
    flag, err := c.L3.GetFlag(name)
    if err == nil {
        // 寫入所有快取層
        flagData, _ := json.Marshal(flag)
        c.L2.Set(ctx, "flag:"+name, flagData, 5*time.Minute)
        c.L1.Store(name, flag)
    }
    return flag, err
}
```

#### 2. 批次評估
```typescript
// 批次載入多個標記，減少網路往返
class FeatureFlagBatch {
  async loadFlags(flagNames: string[], userID?: number): Promise<Record<string, boolean>> {
    const response = await fetch('/api/v1/feature-flags/batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ flags: flagNames, user_id: userID })
    });
    
    return await response.json();
  }
}

// 使用範例
const flags = await featureFlagBatch.loadFlags([
  'new_checkout_flow',
  'enhanced_search', 
  'beta_dashboard'
], userID);
```

---

## 🔧 實作清單

### 開發任務
- [ ] **後端 Feature Flag 服務實作**
  - [ ] Redis 存儲層
  - [ ] REST API 端點
  - [ ] 用戶分群邏輯
  - [ ] A/B 測試分配演算法
  
- [ ] **前端整合**
  - [ ] TypeScript SDK
  - [ ] React Hooks
  - [ ] 組件包裝器
  - [ ] 效能最佳化
  
- [ ] **管理界面**
  - [ ] 標記管理 Dashboard
  - [ ] A/B 測試控制台
  - [ ] 即時監控面板
  - [ ] 審計日誌檢視

### 運維任務
- [ ] **監控設置**
  - [ ] CloudWatch/Stackdriver 指標
  - [ ] 告警規則配置
  - [ ] 效能監控儀表板
  - [ ] 健康檢查端點
  
- [ ] **安全配置**
  - [ ] RBAC 權限控制
  - [ ] API 身份驗證
  - [ ] 審計日誌記錄
  - [ ] 敏感標記保護

### 文檔與培訓
- [ ] **開發者文檔**
  - [ ] API 使用指南
  - [ ] 整合範例程式碼
  - [ ] 最佳實務指南
  - [ ] 故障排除手冊
  
- [ ] **團隊培訓**
  - [ ] Feature Flag 概念培訓
  - [ ] A/B 測試方法論
  - [ ] 安全最佳實務
  - [ ] 應急處理程序

---

**文檔版本**: 1.0  
**最後更新**: 2024-09-03  
**維護人員**: Platform Team  
**審核狀態**: ✅ 已審核  
**下次審查**: 2024-12-03