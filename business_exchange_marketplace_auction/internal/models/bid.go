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
	
	// 英式拍賣專用字段
	MaxProxyAmount *float64   `gorm:"type:decimal(18,2)" json:"max_proxy_amount,omitempty"` // 代理出價上限
	IsWinning      bool       `gorm:"default:false" json:"is_winning"`                      // 是否為當前最高出價
	IsVisible      bool       `gorm:"default:true" json:"is_visible"`                       // 出價是否可見
	
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
	RejectReasonOutOfRange    = "out_of_range"
	RejectReasonTooLate       = "too_late"
	RejectReasonBlacklisted   = "blacklisted"
	RejectReasonTooFrequent   = "too_frequent"
	RejectReasonInvalidAmount = "invalid_amount"
	RejectReasonUnderMinimum  = "under_minimum"
	RejectReasonOutbid        = "outbid"
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

// HasProxyBid 檢查是否為代理出價
func (b *Bid) HasProxyBid() bool {
	return b.MaxProxyAmount != nil && *b.MaxProxyAmount > b.Amount
}

// SetAsWinning 標記為最高出價
func (b *Bid) SetAsWinning() {
	b.IsWinning = true
}

// SetAsNotWinning 標記為非最高出價
func (b *Bid) SetAsNotWinning() {
	b.IsWinning = false
}

// SetVisible 設置出價可見性（英式拍賣為true，密封拍賣在結束前為false）
func (b *Bid) SetVisible(visible bool) {
	b.IsVisible = visible
}

// GetEffectiveAmount 獲取有效出價金額（考慮代理出價）
func (b *Bid) GetEffectiveAmount() float64 {
	if b.MaxProxyAmount != nil && *b.MaxProxyAmount > b.Amount {
		return *b.MaxProxyAmount
	}
	return b.Amount
}

// CanIncreaseToAmount 檢查代理出價是否可以增加到指定金額
func (b *Bid) CanIncreaseToAmount(targetAmount float64) bool {
	return b.HasProxyBid() && *b.MaxProxyAmount >= targetAmount
}

// ExecuteProxyIncrease 執行代理出價增加
func (b *Bid) ExecuteProxyIncrease(newAmount float64) bool {
	if !b.CanIncreaseToAmount(newAmount) {
		return false
	}
	
	b.Amount = newAmount
	return true
}