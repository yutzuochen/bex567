package handlers

import (
	"net/http"
	"strconv"
	"time"

	"auction_service/internal/middleware"
	"auction_service/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ListAuctionsResponse 拍賣列表響應
type ListAuctionsResponse struct {
	Items         []AuctionListItem `json:"items"`
	NextPageToken string            `json:"next_page_token,omitempty"`
}

// AuctionListItem 拍賣列表項目
type AuctionListItem struct {
	AuctionID       uint64     `json:"auction_id"`
	ListingID       uint64     `json:"listing_id"`
	SellerID        uint64     `json:"seller_id"`
	AuctionType     string     `json:"auction_type"`
	StatusCode      string     `json:"status_code"`
	StartAt         time.Time  `json:"start_at"`
	EndAt           time.Time  `json:"end_at"`
	AllowedMinBid   float64    `json:"allowed_min_bid"`
	AllowedMaxBid   float64    `json:"allowed_max_bid"`
	IsAnonymous     bool       `json:"is_anonymous"`
	ExtendedUntil   *time.Time `json:"extended_until,omitempty"`
	ExtensionCount  int        `json:"extension_count"`
	// 英式拍賣特定字段
	CurrentPrice    *float64   `json:"current_price,omitempty"`
	ReserveMet      bool       `json:"reserve_met"`
	Stats           struct {
		Participants int `json:"participants"`
	} `json:"stats"`
}

// ListAuctions 取得拍賣列表 GET /api/v1/auctions
func (h *AuctionHandler) ListAuctions(c *gin.Context) {
	status := c.Query("status")
	city := c.Query("city")
	industry := c.Query("industry")
	sort := c.DefaultQuery("sort", "end_at")
	limitStr := c.DefaultQuery("limit", "20")
	pageToken := c.Query("page_token")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}

	query := h.DB.Model(&models.Auction{})

	// 狀態過濾
	if status != "" {
		query = query.Where("status_code = ?", status)
	}

	// 城市和行業過濾（需要 join listings 表）
	if city != "" || industry != "" {
		// 這裡假設你有 listings 表，需要根據實際情況調整
		query = query.Joins("JOIN listings ON auctions.listing_id = listings.id")
		if city != "" {
			query = query.Where("listings.city = ?", city)
		}
		if industry != "" {
			query = query.Where("listings.industry = ?", industry)
		}
	}

	// 分頁
	if pageToken != "" {
		if pageTokenID, err := strconv.ParseUint(pageToken, 10, 64); err == nil {
			query = query.Where("auction_id > ?", pageTokenID)
		}
	}

	// 排序
	switch sort {
	case "end_at":
		query = query.Order("end_at ASC")
	case "created_at":
		query = query.Order("created_at DESC")
	default:
		query = query.Order("end_at ASC")
	}

	var auctions []models.Auction
	if err := query.Limit(limit + 1).Find(&auctions).Error; err != nil {
		h.Logger.Error("Failed to list auctions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to list auctions",
		}})
		return
	}

	items := make([]AuctionListItem, 0)
	nextPageToken := ""

	for i, auction := range auctions {
		if i >= limit {
			nextPageToken = strconv.FormatUint(auction.AuctionID, 10)
			break
		}

		// 統計參與人數
		var participantCount int64
		h.DB.Model(&models.Bid{}).
			Where("auction_id = ? AND accepted = ? AND deleted_at IS NULL", auction.AuctionID, true).
			Distinct("bidder_id").
			Count(&participantCount)

		item := AuctionListItem{
			AuctionID:       auction.AuctionID,
			ListingID:       auction.ListingID,
			SellerID:        auction.SellerID,
			AuctionType:     string(auction.AuctionType),
			StatusCode:      auction.StatusCode,
			StartAt:         auction.StartAt,
			EndAt:           auction.EndAt,
			AllowedMinBid:   auction.AllowedMinBid,
			AllowedMaxBid:   auction.AllowedMaxBid,
			IsAnonymous:     auction.IsAnonymous,
			ExtendedUntil:   auction.ExtendedUntil,
			ExtensionCount:  auction.ExtensionCount,
			CurrentPrice:    auction.CurrentPrice,
			ReserveMet:      auction.ReserveMet,
		}
		item.Stats.Participants = int(participantCount)

		items = append(items, item)
	}

	c.JSON(http.StatusOK, ListAuctionsResponse{
		Items:         items,
		NextPageToken: nextPageToken,
	})
}

// GetAuctionResponse 單一拍賣響應
type GetAuctionResponse struct {
	Auction AuctionDetail `json:"auction"`
	Viewer  ViewerInfo    `json:"viewer"`
}

// AuctionDetail 拍賣詳情
type AuctionDetail struct {
	AuctionID       uint64     `json:"auction_id"`
	ListingID       uint64     `json:"listing_id"`
	AuctionType     string     `json:"auction_type"`
	StatusCode      string     `json:"status_code"`
	StartAt         time.Time  `json:"start_at"`
	EndAt           time.Time  `json:"end_at"`
	ExtendedUntil   *time.Time `json:"extended_until,omitempty"`
	ExtensionCount  int        `json:"extension_count"`
	AllowedMinBid   float64    `json:"allowed_min_bid"`
	AllowedMaxBid   float64    `json:"allowed_max_bid"`
	IsAnonymous     bool       `json:"is_anonymous"`
	// 英式拍賣特定字段
	CurrentPrice    *float64   `json:"current_price,omitempty"`
	ReserveMet      bool       `json:"reserve_met"`
	ReservePrice    *float64   `json:"reserve_price,omitempty"`
	MinIncrement    float64    `json:"min_increment"`
	BuyItNow        *float64   `json:"buy_it_now,omitempty"`
}

// ViewerInfo 觀看者資訊
type ViewerInfo struct {
	CanBid      bool   `json:"can_bid"`
	AliasLabel  string `json:"alias_label,omitempty"`
	Blacklisted bool   `json:"blacklisted"`
}

// GetAuction 取得單一拍賣 GET /api/v1/auctions/:id
func (h *AuctionHandler) GetAuction(c *gin.Context) {
	auctionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid auction ID",
		}})
		return
	}

	var auction models.Auction
	if err := h.DB.First(&auction, auctionID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
				"code":    "not_found",
				"message": "Auction not found",
			}})
			return
		}
		h.Logger.Error("Failed to find auction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to find auction",
		}})
		return
	}

	// 增加瀏覽次數
	h.DB.Model(&auction).Update("view_count", gorm.Expr("view_count + 1"))

	auctionDetail := AuctionDetail{
		AuctionID:       auction.AuctionID,
		ListingID:       auction.ListingID,
		AuctionType:     string(auction.AuctionType),
		StatusCode:      auction.StatusCode,
		StartAt:         auction.StartAt,
		EndAt:           auction.EndAt,
		ExtendedUntil:   auction.ExtendedUntil,
		ExtensionCount:  auction.ExtensionCount,
		AllowedMinBid:   auction.AllowedMinBid,
		AllowedMaxBid:   auction.AllowedMaxBid,
		IsAnonymous:     auction.IsAnonymous,
		CurrentPrice:    auction.CurrentPrice,
		ReserveMet:      auction.ReserveMet,
		ReservePrice:    auction.ReservePrice,
		MinIncrement:    auction.MinIncrement,
		BuyItNow:        auction.BuyItNow,
	}

	viewer := ViewerInfo{
		CanBid:      false,
		Blacklisted: false,
	}

	// 檢查用戶狀態（如果已登入）
	if userID, exists := c.Get(middleware.UserIDKey); exists {
		userIDValue := userID.(uint64)

		// 檢查黑名單
		var blacklist models.UserBlacklist
		if err := h.DB.Where("user_id = ? AND is_active = ?", userIDValue, true).First(&blacklist).Error; err == nil {
			viewer.Blacklisted = true
		} else if err != gorm.ErrRecordNotFound {
			h.Logger.Error("Failed to check blacklist", zap.Error(err))
		}

		// 設定是否可以出價
		viewer.CanBid = !viewer.Blacklisted && auction.IsActive()

		// 取得匿名別名（如果存在）
		var alias models.AuctionBidderAlias
		if err := h.DB.Where("auction_id = ? AND bidder_id = ?", auctionID, userIDValue).First(&alias).Error; err == nil {
			viewer.AliasLabel = alias.AliasLabel
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": GetAuctionResponse{
			Auction: auctionDetail,
			Viewer:  viewer,
		},
	})
}