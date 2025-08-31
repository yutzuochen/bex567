package models

import (
	"encoding/json"
	"time"
)

// AuctionStatusHistory 拍賣狀態歷史
type AuctionStatusHistory struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AuctionID   uint64     `gorm:"not null;index" json:"auction_id"`
	FromStatus  string     `gorm:"size:16;not null" json:"from_status"`
	ToStatus    string     `gorm:"size:16;not null" json:"to_status"`
	Reason      string     `gorm:"size:255" json:"reason,omitempty"`
	OperatorID  *uint64    `json:"operator_id,omitempty"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`

	// 關聯
	Auction *Auction `gorm:"foreignKey:AuctionID" json:"auction,omitempty"`
}

func (AuctionStatusHistory) TableName() string {
	return "auction_status_history"
}

// EventType 事件類型
type EventType string

const (
	EventTypeOpen        EventType = "open"
	EventTypeBidAccepted EventType = "bid_accepted"
	EventTypeBidRejected EventType = "bid_rejected"
	EventTypeExtended    EventType = "extended"
	EventTypeClosed      EventType = "closed"
	EventTypeNotified    EventType = "notified"
	EventTypeError       EventType = "error"
)

// AuctionEvent 拍賣事件（WS 對帳、斷線恢復、審計）
type AuctionEvent struct {
	EventID     uint64          `gorm:"primaryKey;autoIncrement" json:"event_id"`
	AuctionID   uint64          `gorm:"not null;index" json:"auction_id"`
	EventType   EventType       `gorm:"type:enum('open','bid_accepted','bid_rejected','extended','closed','notified','error')" json:"event_type"`
	ActorUserID *uint64         `json:"actor_user_id,omitempty"`
	Payload     json.RawMessage `gorm:"type:json" json:"payload,omitempty"`
	CreatedAt   time.Time       `gorm:"autoCreateTime" json:"created_at"`

	// 關聯
	Auction *Auction `gorm:"foreignKey:AuctionID" json:"auction,omitempty"`
}

func (AuctionEvent) TableName() string {
	return "auction_events"
}

// SetPayload 設定 payload
func (e *AuctionEvent) SetPayload(data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	e.Payload = payload
	return nil
}

// GetPayload 取得 payload
func (e *AuctionEvent) GetPayload(target interface{}) error {
	return json.Unmarshal(e.Payload, target)
}

// AuctionBidderAlias 匿名別名（Bidder #23）
type AuctionBidderAlias struct {
	AuctionID  uint64    `gorm:"primaryKey" json:"auction_id"`
	BidderID   uint64    `gorm:"primaryKey" json:"bidder_id"`
	AliasNum   int       `gorm:"not null" json:"alias_num"`
	AliasLabel string    `gorm:"size:32;not null;uniqueIndex:uk_alias_label" json:"alias_label"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	// 關聯
	Auction *Auction `gorm:"foreignKey:AuctionID" json:"auction,omitempty"`
}

func (AuctionBidderAlias) TableName() string {
	return "auction_bidder_aliases"
}

// AuctionBidHistogram 出價分佈快照
type AuctionBidHistogram struct {
	AuctionID   uint64    `gorm:"primaryKey" json:"auction_id"`
	BucketLow   float64   `gorm:"primaryKey;type:decimal(18,2)" json:"bucket_low"`
	BucketHigh  float64   `gorm:"primaryKey;type:decimal(18,2)" json:"bucket_high"`
	ComputedAt  time.Time `gorm:"primaryKey" json:"computed_at"`
	BidCount    int       `gorm:"not null" json:"bid_count"`

	// 關聯
	Auction *Auction `gorm:"foreignKey:AuctionID" json:"auction,omitempty"`
}

func (AuctionBidHistogram) TableName() string {
	return "auction_bid_histograms"
}

// AuctionStreamOffset WS 斷線恢復
type AuctionStreamOffset struct {
	AuctionID   uint64    `gorm:"primaryKey" json:"auction_id"`
	UserID      uint64    `gorm:"primaryKey" json:"user_id"`
	LastEventID uint64    `gorm:"not null" json:"last_event_id"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AuctionStreamOffset) TableName() string {
	return "auction_stream_offsets"
}