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

type BlacklistHandler struct {
	DB     *gorm.DB
	Logger *zap.Logger
}

// ListBlacklistResponse 黑名單列表響應
type ListBlacklistResponse struct {
	Items []BlacklistItem `json:"items"`
}

// BlacklistItem 黑名單項目
type BlacklistItem struct {
	UserID    uint64 `json:"user_id"`
	IsActive  bool   `json:"is_active"`
	Reason    string `json:"reason,omitempty"`
	StaffID   *uint64 `json:"staff_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AddBlacklistRequest 新增黑名單請求
type AddBlacklistRequest struct {
	UserID uint64 `json:"user_id" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

// ListBlacklist 取得黑名單 GET /api/v1/admin/blacklist
func (h *BlacklistHandler) ListBlacklist(c *gin.Context) {
	activeOnly := c.DefaultQuery("active", "true") == "true"
	limitStr := c.DefaultQuery("limit", "50")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 50
	}

	query := h.DB.Model(&models.UserBlacklist{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	var blacklists []models.UserBlacklist
	if err := query.Limit(limit).Order("created_at DESC").Find(&blacklists).Error; err != nil {
		h.Logger.Error("Failed to list blacklist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to list blacklist",
		}})
		return
	}

	items := make([]BlacklistItem, 0)
	for _, bl := range blacklists {
		items = append(items, BlacklistItem{
			UserID:    bl.UserID,
			IsActive:  bl.IsActive,
			Reason:    bl.Reason,
			StaffID:   bl.StaffID,
			CreatedAt: bl.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	c.JSON(http.StatusOK, ListBlacklistResponse{
		Items: items,
	})
}

// AddBlacklist 新增黑名單 POST /api/v1/admin/blacklist
func (h *BlacklistHandler) AddBlacklist(c *gin.Context) {
	var req AddBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid request format",
			"details": err.Error(),
		}})
		return
	}

	staffID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"code":    "unauthorized",
			"message": "User not authenticated",
		}})
		return
	}

	staffIDValue := staffID.(uint64)

	// 檢查用戶是否已經在黑名單中
	var existingBlacklist models.UserBlacklist
	err := h.DB.Where("user_id = ?", req.UserID).First(&existingBlacklist).Error
	
	if err == nil {
		// 已存在，更新狀態
		existingBlacklist.Activate(req.Reason, staffIDValue)
		if err := h.DB.Save(&existingBlacklist).Error; err != nil {
			h.Logger.Error("Failed to update blacklist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"code":    "internal_error",
				"message": "Failed to update blacklist",
			}})
			return
		}
	} else if err == gorm.ErrRecordNotFound {
		// 不存在，創建新記錄
		blacklist := &models.UserBlacklist{
			UserID:   req.UserID,
			IsActive: true,
			Reason:   req.Reason,
			StaffID:  &staffIDValue,
		}

		if err := h.DB.Create(blacklist).Error; err != nil {
			h.Logger.Error("Failed to create blacklist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"code":    "internal_error",
				"message": "Failed to create blacklist",
			}})
			return
		}

		existingBlacklist = *blacklist
	} else {
		h.Logger.Error("Failed to check existing blacklist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to process request",
		}})
		return
	}

	// 記錄審計日誌
	auditLog := models.NewAuditLog(
		&staffIDValue,
		models.ActionBlacklistAdd,
		models.EntityTypeBlacklist,
		existingBlacklist.UserID,
		existingBlacklist,
	)
	h.DB.Create(auditLog)

	c.JSON(http.StatusOK, gin.H{
		"message": "User added to blacklist successfully",
		"user_id": req.UserID,
	})
}

// RemoveBlacklist 移除黑名單 DELETE /api/v1/admin/blacklist/:user_id
func (h *BlacklistHandler) RemoveBlacklist(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"code":    "bad_request",
			"message": "Invalid user ID",
		}})
		return
	}

	staffID, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{
			"code":    "unauthorized",
			"message": "User not authenticated",
		}})
		return
	}

	var blacklist models.UserBlacklist
	if err := h.DB.Where("user_id = ?", userID).First(&blacklist).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
				"code":    "not_found",
				"message": "User not found in blacklist",
			}})
			return
		}
		h.Logger.Error("Failed to find blacklist entry", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to find blacklist entry",
		}})
		return
	}

	// 停用黑名單
	blacklist.Deactivate()
	if err := h.DB.Save(&blacklist).Error; err != nil {
		h.Logger.Error("Failed to deactivate blacklist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code":    "internal_error",
			"message": "Failed to remove from blacklist",
		}})
		return
	}

	// 記錄審計日誌
	staffIDValue := staffID.(uint64)
	auditLog := models.NewAuditLog(
		&staffIDValue,
		models.ActionBlacklistRemove,
		models.EntityTypeBlacklist,
		userID,
		blacklist,
	)
	h.DB.Create(auditLog)

	c.JSON(http.StatusOK, gin.H{
		"message": "User removed from blacklist successfully",
		"user_id": userID,
	})
}