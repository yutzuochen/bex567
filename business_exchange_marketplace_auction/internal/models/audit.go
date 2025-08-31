package models

import (
	"encoding/json"
	"time"
)

// AuditLog 審計日誌 - 匹配 business_exchange 數據庫現有結構
type AuditLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    *uint64   `json:"user_id,omitempty"`
	Event     string    `gorm:"size:100;not null" json:"event"`
	Details   *string   `gorm:"type:text" json:"details,omitempty"`
	IPAddress *string   `gorm:"size:45" json:"ip_address,omitempty"`
	UserAgent *string   `gorm:"size:500" json:"user_agent,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

// Action constants
const (
	ActionAuctionCreate   = "AUCTION_CREATE"
	ActionAuctionActivate = "AUCTION_ACTIVATE"
	ActionAuctionCancel   = "AUCTION_CANCEL"
	ActionAuctionExtend   = "AUCTION_EXTEND"
	ActionAuctionClose    = "AUCTION_CLOSE"
	ActionBidPlace        = "BID_PLACE"
	ActionBidReject       = "BID_REJECT"
	ActionBidDelete       = "BID_DELETE"
	ActionBlacklistAdd    = "BLACKLIST_ADD"
	ActionBlacklistRemove = "BLACKLIST_REMOVE"
)

// Entity types
const (
	EntityTypeAuction   = "auction"
	EntityTypeBid       = "bid"
	EntityTypeUser      = "user"
	EntityTypeBlacklist = "blacklist"
)

// SetDetails 設定詳細資訊為 JSON
func (a *AuditLog) SetDetails(data interface{}) error {
	detailsJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	detailsStr := string(detailsJSON)
	a.Details = &detailsStr
	return nil
}

// GetDetails 取得詳細資訊
func (a *AuditLog) GetDetails(target interface{}) error {
	if a.Details == nil {
		return nil
	}
	return json.Unmarshal([]byte(*a.Details), target)
}

// NewAuditLog 創建新的審計日誌 - 兼容舊版本API
func NewAuditLog(userID *uint64, action, entityType string, entityID uint64, entityData interface{}) *AuditLog {
	auditLog := &AuditLog{
		UserID: userID,
		Event:  action,
	}
	
	auditLog.SetDetails(map[string]interface{}{
		"action":      action,
		"entity_type": entityType,
		"entity_id":   entityID,
		"entity_data": entityData,
	})
	
	return auditLog
}