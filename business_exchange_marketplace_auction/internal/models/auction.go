package models

import (
	"time"
)

// AuctionStatusRef 拍賣狀態參考表
type AuctionStatusRef struct {
	StatusCode  string `gorm:"primaryKey;size:16" json:"status_code"`
	IsOpen      bool   `gorm:"not null" json:"is_open"`
	Description string `gorm:"size:255;not null" json:"description"`
}

func (AuctionStatusRef) TableName() string {
	return "auction_status_ref"
}

// AuctionType 拍賣類型
type AuctionType string

const (
	AuctionTypeSealed  AuctionType = "sealed"
	AuctionTypeEnglish AuctionType = "english"
	AuctionTypeDutch   AuctionType = "dutch"
)

// AuctionStatus 拍賣狀態
type AuctionStatus string

const (
	AuctionStatusDraft     AuctionStatus = "draft"
	AuctionStatusActive    AuctionStatus = "active"
	AuctionStatusExtended  AuctionStatus = "extended"
	AuctionStatusEnded     AuctionStatus = "ended"
	AuctionStatusCancelled AuctionStatus = "cancelled"
)

// Auction 拍賣主表
type Auction struct {
	AuctionID           uint64        `gorm:"primaryKey;autoIncrement" json:"auction_id"`
	ListingID           uint64        `gorm:"not null;index" json:"listing_id"` // References listings.id
	SellerID            uint64        `gorm:"not null;index" json:"seller_id"` // Must match listings.owner_id
	AuctionType         AuctionType   `gorm:"type:enum('sealed','english','dutch');default:'sealed'" json:"auction_type"`
	StatusCode          string        `gorm:"size:16;not null" json:"status_code"`
	AllowedMinBid       float64       `gorm:"type:decimal(18,2);not null" json:"allowed_min_bid"`
	AllowedMaxBid       float64       `gorm:"type:decimal(18,2);not null" json:"allowed_max_bid"`
	SoftCloseTriggerSec int           `gorm:"default:180" json:"soft_close_trigger_sec"`
	SoftCloseExtendSec  int           `gorm:"default:60" json:"soft_close_extend_sec"`
	StartAt             time.Time     `gorm:"not null" json:"start_at"`
	EndAt               time.Time     `gorm:"not null" json:"end_at"`
	ExtendedUntil       *time.Time    `json:"extended_until"`
	ExtensionCount      int           `gorm:"default:0" json:"extension_count"`
	IsAnonymous         bool          `gorm:"default:true" json:"is_anonymous"`
	ViewCount           int           `gorm:"default:0" json:"view_count"`
	CreatedAt           time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time     `gorm:"autoUpdateTime" json:"updated_at"`

	// 關聯 (foreign keys handled manually in migrations)
	StatusRef AuctionStatusRef `gorm:"-" json:"status_ref,omitempty"`
	Bids      []Bid            `gorm:"-" json:"bids,omitempty"`
	Listing   *Listing         `gorm:"foreignKey:ListingID" json:"listing,omitempty"`
}

func (Auction) TableName() string {
	return "auctions"
}

// IsActive 檢查拍賣是否為活躍狀態
func (a *Auction) IsActive() bool {
	return a.StatusCode == string(AuctionStatusActive) || a.StatusCode == string(AuctionStatusExtended)
}

// GetEffectiveEndTime 取得有效的結束時間（考慮延長）
func (a *Auction) GetEffectiveEndTime() time.Time {
	if a.ExtendedUntil != nil && a.ExtendedUntil.After(a.EndAt) {
		return *a.ExtendedUntil
	}
	return a.EndAt
}

// IsInSoftCloseWindow 檢查是否在軟關閉視窗內
func (a *Auction) IsInSoftCloseWindow() bool {
	if !a.IsActive() {
		return false
	}
	
	effectiveEnd := a.GetEffectiveEndTime()
	triggerTime := effectiveEnd.Add(-time.Duration(a.SoftCloseTriggerSec) * time.Second)
	
	return time.Now().After(triggerTime)
}

// CanExtend 檢查是否可以延長
func (a *Auction) CanExtend() bool {
	return a.IsActive() && a.IsInSoftCloseWindow()
}

// ExtendAuction 延長拍賣時間
func (a *Auction) ExtendAuction() {
	if !a.CanExtend() {
		return
	}
	
	currentEnd := a.GetEffectiveEndTime()
	newEnd := currentEnd.Add(time.Duration(a.SoftCloseExtendSec) * time.Second)
	a.ExtendedUntil = &newEnd
	a.ExtensionCount++
	
	if a.StatusCode == string(AuctionStatusActive) {
		a.StatusCode = string(AuctionStatusExtended)
	}
}

// ValidateBidAmount 驗證出價金額是否在允許範圍內
func (a *Auction) ValidateBidAmount(amount float64) bool {
	return amount >= a.AllowedMinBid && amount <= a.AllowedMaxBid
}

// ValidateOwnership 驗證拍賣的賣家是否為商品的擁有者
// 這確保只有商品擁有者才能為其商品創建拍賣
func (a *Auction) ValidateOwnership(listing *Listing) bool {
	return a.SellerID == uint64(listing.OwnerID)
}