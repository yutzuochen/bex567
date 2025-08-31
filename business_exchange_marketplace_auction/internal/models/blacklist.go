package models

import (
	"time"
)

// UserBlacklist 黑名單（全站）
type UserBlacklist struct {
	UserID    uint64    `gorm:"primaryKey" json:"user_id"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	Reason    string    `gorm:"size:255" json:"reason,omitempty"`
	StaffID   *uint64   `json:"staff_id,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserBlacklist) TableName() string {
	return "user_blacklist"
}

// IsBlacklisted 檢查用戶是否被列入黑名單
func (b *UserBlacklist) IsBlacklisted() bool {
	return b.IsActive
}

// Activate 啟用黑名單
func (b *UserBlacklist) Activate(reason string, staffID uint64) {
	b.IsActive = true
	b.Reason = reason
	b.StaffID = &staffID
}

// Deactivate 停用黑名單
func (b *UserBlacklist) Deactivate() {
	b.IsActive = false
}