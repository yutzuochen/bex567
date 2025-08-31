package models

import (
	"encoding/json"
	"time"
)

// NotificationKind 通知類型
type NotificationKind string

const (
	NotificationKindWinner         NotificationKind = "winner"
	NotificationKindSellerResult   NotificationKind = "seller_result"
	NotificationKindTop7           NotificationKind = "top7"
	NotificationKindParticipantEnd NotificationKind = "participant_end"
)

// NotificationChannel 通知頻道
type NotificationChannel string

const (
	NotificationChannelEmail   NotificationChannel = "email"
	NotificationChannelSMS     NotificationChannel = "sms"
	NotificationChannelLine    NotificationChannel = "line"
	NotificationChannelWebPush NotificationChannel = "webpush"
)

// NotificationStatus 通知狀態
type NotificationStatus string

const (
	NotificationStatusQueued NotificationStatus = "queued"
	NotificationStatusSent   NotificationStatus = "sent"
	NotificationStatusFailed NotificationStatus = "failed"
)

// AuctionNotificationLog 通知紀錄
type AuctionNotificationLog struct {
	ID        uint64              `gorm:"primaryKey;autoIncrement" json:"id"`
	AuctionID uint64              `gorm:"not null;index;uniqueIndex:uk_once" json:"auction_id"`
	UserID    uint64              `gorm:"not null;uniqueIndex:uk_once" json:"user_id"`
	Kind      NotificationKind    `gorm:"type:enum('winner','seller_result','top7','participant_end');uniqueIndex:uk_once" json:"kind"`
	Channel   NotificationChannel `gorm:"type:enum('email','sms','line','webpush')" json:"channel"`
	Status    NotificationStatus  `gorm:"type:enum('queued','sent','failed');default:'queued'" json:"status"`
	Meta      json.RawMessage     `gorm:"type:json" json:"meta,omitempty"`
	CreatedAt time.Time           `gorm:"autoCreateTime" json:"created_at"`

	// 關聯
	Auction *Auction `gorm:"foreignKey:AuctionID" json:"auction,omitempty"`
}

func (AuctionNotificationLog) TableName() string {
	return "auction_notification_log"
}

// SetMeta 設定 meta 資料
func (n *AuctionNotificationLog) SetMeta(data interface{}) error {
	meta, err := json.Marshal(data)
	if err != nil {
		return err
	}
	n.Meta = meta
	return nil
}

// GetMeta 取得 meta 資料
func (n *AuctionNotificationLog) GetMeta(target interface{}) error {
	return json.Unmarshal(n.Meta, target)
}

// MarkAsSent 標記為已發送
func (n *AuctionNotificationLog) MarkAsSent() {
	n.Status = NotificationStatusSent
}

// MarkAsFailed 標記為失敗
func (n *AuctionNotificationLog) MarkAsFailed() {
	n.Status = NotificationStatusFailed
}