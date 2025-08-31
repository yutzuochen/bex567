package handlers

import (
	"net/http"
	"strconv"

	"auction_service/internal/middleware"
	"auction_service/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MyBidsResponse 我的出價響應
type MyBidsResponse struct {
	Items         []BidItem `json:"items"`
	NextPageToken string    `json:"next_page_token,omitempty"`
}

// BidItem 出價項目
type BidItem struct {
	BidID        uint64  `json:"bid_id"`
	Amount       float64 `json:"amount"`
	Accepted     bool    `json:"accepted"`
	RejectReason string  `json:"reject_reason,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// GetMyBids 查詢我的出價 GET /api/v1/auctions/:id/my-bids
func (h *BidHandler) GetMyBids(c *gin.Context) {
	auctionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid auction ID",
		}})
		return
	}

	userID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"code":    "unauthorized",
			"message": "User not authenticated",
		}})
		return
	}

	limitStr := c.DefaultQuery("limit", "20")
	pageToken := c.Query("page_token")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 50 {
		limit = 20
	}

	query := h.DB.Where("auction_id = ? AND bidder_id = ? AND deleted_at IS NULL", 
		auctionID, userID.(uint64))

	// 分頁
	if pageToken != "" {
		if pageTokenID, err := strconv.ParseUint(pageToken, 10, 64); err == nil {
			query = query.Where("bid_id < ?", pageTokenID)
		}
	}

	var bids []models.Bid
	if err := query.Order("created_at DESC").Limit(limit + 1).Find(&bids).Error; err != nil {
		h.Logger.Error("Failed to get user bids", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to get bids",
		}})
		return
	}

	items := make([]BidItem, 0)
	nextPageToken := ""

	for i, bid := range bids {
		if i >= limit {
			nextPageToken = strconv.FormatUint(bid.BidID, 10)
			break
		}

		item := BidItem{
			BidID:        bid.BidID,
			Amount:       bid.Amount,
			Accepted:     bid.Accepted,
			RejectReason: bid.RejectReason,
			CreatedAt:    bid.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": MyBidsResponse{
			Items:         items,
			NextPageToken: nextPageToken,
		},
	})
}

// AuctionResultsResponse 拍賣結果響應
type AuctionResultsResponse struct {
	Items []ResultItem `json:"items"`
	TopK  int          `json:"top_k"`
}

// ResultItem 結果項目
type ResultItem struct {
	FinalRank    int     `json:"final_rank"`
	BidderAlias  string  `json:"bidder_alias"`
	Amount       float64 `json:"amount"`
}

// GetAuctionResults 取得拍賣結果 GET /api/v1/auctions/:id/results
func (h *BidHandler) GetAuctionResults(c *gin.Context) {
	auctionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid auction ID",
		}})
		return
	}

	// 檢查拍賣是否存在且已結束
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

	// 只有已結束的拍賣才能查看結果
	if auction.StatusCode != string(models.AuctionStatusEnded) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{
			"code":    "auction_not_ended",
			"message": "Results only available for ended auctions",
		}})
		return
	}

	// 檢查權限：賣家、管理員或得標前7名可查看
	userID, exists := c.Get(middleware.UserIDKey)
	userRole, _ := c.Get(middleware.UserRoleKey)
	
	canView := false
	if exists {
		userIDValue := userID.(uint64)
		// 賣家或管理員可以查看
		if auction.SellerID == userIDValue || userRole == "admin" {
			canView = true
		} else {
			// 檢查是否為前7名
			var userBid models.Bid
			if err := h.DB.Where("auction_id = ? AND bidder_id = ? AND accepted = ? AND deleted_at IS NULL AND final_rank IS NOT NULL AND final_rank <= ?", 
				auctionID, userIDValue, true, 7).First(&userBid).Error; err == nil {
				canView = true
			}
		}
	}

	if !canView {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"code":    "forbidden",
			"message": "Only seller, admin, or top 7 bidders can view results",
		}})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 50
	}

	// 取得排名結果，join 別名表
	var results []struct {
		FinalRank   int     `json:"final_rank"`
		Amount      float64 `json:"amount"`
		AliasLabel  string  `json:"alias_label"`
	}

	if err := h.DB.Table("bids").
		Select("bids.final_rank, bids.amount, COALESCE(auction_bidder_aliases.alias_label, CONCAT('Bidder #', bids.bidder_id)) as alias_label").
		Joins("LEFT JOIN auction_bidder_aliases ON bids.auction_id = auction_bidder_aliases.auction_id AND bids.bidder_id = auction_bidder_aliases.bidder_id").
		Where("bids.auction_id = ? AND bids.accepted = ? AND bids.deleted_at IS NULL AND bids.final_rank IS NOT NULL", auctionID, true).
		Order("bids.final_rank ASC").
		Limit(limit).
		Find(&results).Error; err != nil {
		h.Logger.Error("Failed to get auction results", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to get auction results",
		}})
		return
	}

	items := make([]ResultItem, 0)
	for _, result := range results {
		items = append(items, ResultItem{
			FinalRank:   result.FinalRank,
			BidderAlias: result.AliasLabel,
			Amount:      result.Amount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": AuctionResultsResponse{
			Items: items,
			TopK:  7,
		},
	})
}

// GetHistogramResponse 分佈圖響應
type GetHistogramResponse struct {
	ComputedAt     string           `json:"computed_at"`
	Buckets        []HistogramBucket `json:"buckets"`
	KAnonymityMin  int              `json:"k_anonymity_min"`
}

// HistogramBucket 分佈桶
type HistogramBucket struct {
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
	Count int     `json:"count"`
}

// GetAuctionHistogram 取得出價分佈 GET /api/v1/auctions/:id/stats/histogram
func (h *BidHandler) GetAuctionHistogram(c *gin.Context) {
	auctionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid auction ID",
		}})
		return
	}

	atParam := c.Query("at")
	
	query := h.DB.Where("auction_id = ?", auctionID)
	
	// 如果指定時間，則查找該時間點的快照
	if atParam != "" {
		// 這裡可以解析時間參數，暫時忽略
	}

	// 取得最新的分佈快照
	var histograms []models.AuctionBidHistogram
	if err := query.Order("computed_at DESC").Limit(20).Find(&histograms).Error; err != nil {
		h.Logger.Error("Failed to get histogram", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to get bid distribution",
		}})
		return
	}

	if len(histograms) == 0 {
		c.JSON(http.StatusOK, GetHistogramResponse{
			Buckets:       []HistogramBucket{},
			KAnonymityMin: 5,
		})
		return
	}

	buckets := make([]HistogramBucket, 0)
	computedAt := histograms[0].ComputedAt

	for _, h := range histograms {
		// 只顯示出價數量 >= 5 的桶（k-anonymity）
		if h.BidCount >= 5 {
			buckets = append(buckets, HistogramBucket{
				Low:   h.BucketLow,
				High:  h.BucketHigh,
				Count: h.BidCount,
			})
		}
	}

	c.JSON(http.StatusOK, GetHistogramResponse{
		ComputedAt:    computedAt.Format("2006-01-02T15:04:05Z07:00"),
		Buckets:       buckets,
		KAnonymityMin: 5,
	})
}