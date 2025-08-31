package models

import (
	"time"
)

// Bid 出價表
type Bid struct {
	BidID         uint64     `gorm:"primaryKey;autoIncrement" json:"bid_id"`
	AuctionID     uint64     `gorm:"not null;index" json:"auction_id"`
	BidderID      uint64     `gorm:"not null;index" json:"bidder_id"`
	Amount        float64    `gorm:"type:decimal(18,2);not null" json:"amount"`
	ClientSeq     int64      `gorm:"not null" json:"client_seq"`
	SourceIPHash  []byte     `gorm:"type:varbinary(32)" json:"source_ip_hash,omitempty"`
	UserAgentHash []byte     `gorm:"type:varbinary(32)" json:"user_agent_hash,omitempty"`
	Accepted      bool       `gorm:"default:true" json:"accepted"`
	RejectReason  string     `gorm:"size:64" json:"reject_reason,omitempty"`
	FinalRank     *int       `json:"final_rank,omitempty"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	DeletedBy     *uint64    `json:"deleted_by,omitempty"`

	// 關聯
	Auction *Auction `gorm:"foreignKey:AuctionID" json:"auction,omitempty"`
}

func (Bid) TableName() string {
	return "bids"
}

// RejectReason constants
const (
	RejectReasonOutOfRange  = "out_of_range"
	RejectReasonTooLate     = "too_late"
	RejectReasonBlacklisted = "blacklisted"
	RejectReasonTooFrequent = "too_frequent"
	RejectReasonInvalidAmount = "invalid_amount"
)

// IsDeleted 檢查出價是否已被軟刪除
func (b *Bid) IsDeleted() bool {
	return b.DeletedAt != nil
}

// SoftDelete 軟刪除出價
func (b *Bid) SoftDelete(deletedBy uint64) {
	now := time.Now()
	b.DeletedAt = &now
	b.DeletedBy = &deletedBy
}

// IsValid 檢查出價是否有效（被接受且未刪除）
func (b *Bid) IsValid() bool {
	return b.Accepted && !b.IsDeleted()
}