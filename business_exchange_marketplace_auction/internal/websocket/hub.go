package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"auction_service/internal/config"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Hub WebSocket 連接管理中心
type Hub struct {
	// 按拍賣 ID 分組的連接
	AuctionRooms map[uint64]map[*Connection]bool
	
	// 連接管理頻道
	Register   chan *Connection
	Unregister chan *Connection
	
	// 廣播頻道
	Broadcast chan *BroadcastMessage
	
	// 依賴項
	DB     *gorm.DB
	Redis  *redis.Client
	Logger *zap.Logger
	Config *config.Config
	
	// 統計資訊
	Stats *HubStats
	
	// 降級管理器
	DegradationMgr *DegradationManager
	
	// 互斥鎖
	mutex sync.RWMutex
}

// BroadcastMessage 廣播訊息
type BroadcastMessage struct {
	AuctionID   uint64
	Message     Message
	ExcludeUser *uint64  // 排除的用戶（例如出價者自己）
	TargetUsers []uint64 // 目標用戶列表（空表示所有用戶）
}

// HubStats Hub 統計資訊
type HubStats struct {
	TotalConnections  int            `json:"total_connections"`
	AuctionRoomCount  int            `json:"auction_room_count"`
	AuctionRoomStats  map[uint64]int `json:"auction_room_stats"`
	DegradedLevel     int            `json:"degraded_level"`
	LastUpdated       time.Time      `json:"last_updated"`
}

// NewHub 創建新的 Hub
func NewHub(db *gorm.DB, redis *redis.Client, logger *zap.Logger, config *config.Config) *Hub {
	return &Hub{
		AuctionRooms: make(map[uint64]map[*Connection]bool),
		Register:     make(chan *Connection, 256),
		Unregister:   make(chan *Connection, 256),
		Broadcast:    make(chan *BroadcastMessage, 1024),
		DB:           db,
		Redis:        redis,
		Logger:       logger,
		Config:       config,
		Stats: &HubStats{
			AuctionRoomStats: make(map[uint64]int),
			LastUpdated:      time.Now(),
		},
		DegradationMgr: NewDegradationManager(logger),
	}
}

// Run 啟動 Hub
func (h *Hub) Run(ctx context.Context) {
	// 啟動統計更新協程
	go h.updateStats(ctx)
	
	// 啟動降級監控協程
	go h.monitorDegradedLevel(ctx)
	
	// 啟動降級管理器清理協程
	go h.cleanupDegradationLimiters(ctx)
	
	// 如果有 Redis，啟動發布訂閱
	if h.Redis != nil {
		go h.listenRedisPubSub(ctx)
	}
	
	for {
		select {
		case conn := <-h.Register:
			h.registerConnection(conn)
			
		case conn := <-h.Unregister:
			h.unregisterConnection(conn)
			
		case broadcast := <-h.Broadcast:
			h.broadcastMessage(broadcast)
			
		case <-ctx.Done():
			h.Logger.Info("Hub shutting down")
			h.closeAllConnections()
			return
		}
	}
}

// registerConnection 註冊新連接
func (h *Hub) registerConnection(conn *Connection) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	
	// 確保拍賣房間存在
	if h.AuctionRooms[conn.AuctionID] == nil {
		h.AuctionRooms[conn.AuctionID] = make(map[*Connection]bool)
	}
	
	// 檢查同一用戶的連接數限制
	userConnCount := h.countUserConnections(conn.AuctionID, conn.UserID)
	if userConnCount >= 3 {
		h.Logger.Warn("User connection limit exceeded",
			zap.Uint64("user_id", conn.UserID),
			zap.Uint64("auction_id", conn.AuctionID),
		)
		conn.SendMessage(Message{
			Type: MessageTypeError,
			Data: map[string]interface{}{
				"code":    "connection_limit",
				"message": "Too many connections for this user",
			},
		})
		conn.Close()
		return
	}
	
	h.AuctionRooms[conn.AuctionID][conn] = true
	
	h.Logger.Info("WebSocket connection registered",
		zap.String("connection_id", conn.ID),
		zap.Uint64("user_id", conn.UserID),
		zap.Uint64("auction_id", conn.AuctionID),
	)
}

// unregisterConnection 註銷連接
func (h *Hub) unregisterConnection(conn *Connection) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	
	if room, ok := h.AuctionRooms[conn.AuctionID]; ok {
		if _, ok := room[conn]; ok {
			delete(room, conn)
			close(conn.Send)
			
			// 如果房間空了，刪除房間
			if len(room) == 0 {
				delete(h.AuctionRooms, conn.AuctionID)
			}
		}
	}
	
	h.Logger.Info("WebSocket connection unregistered",
		zap.String("connection_id", conn.ID),
		zap.Uint64("user_id", conn.UserID),
		zap.Uint64("auction_id", conn.AuctionID),
	)
}

// broadcastMessage 廣播訊息
func (h *Hub) broadcastMessage(broadcast *BroadcastMessage) {
	h.mutex.RLock()
	room, ok := h.AuctionRooms[broadcast.AuctionID]
	h.mutex.RUnlock()
	
	if !ok {
		return
	}
	
	// 準備訊息資料
	messageData, err := json.Marshal(broadcast.Message)
	if err != nil {
		h.Logger.Error("Failed to marshal broadcast message", zap.Error(err))
		return
	}
	
	// 發送給房間內的連接
	for conn := range room {
		// 檢查是否排除此用戶
		if broadcast.ExcludeUser != nil && conn.UserID == *broadcast.ExcludeUser {
			continue
		}
		
		// 檢查是否在目標用戶列表中
		if len(broadcast.TargetUsers) > 0 {
			found := false
			for _, targetUser := range broadcast.TargetUsers {
				if conn.UserID == targetUser {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		
		select {
		case conn.Send <- messageData:
		default:
			h.Logger.Warn("Connection send channel full, closing",
				zap.String("connection_id", conn.ID),
			)
			close(conn.Send)
			delete(room, conn)
		}
	}
}

// countUserConnections 計算用戶連接數
func (h *Hub) countUserConnections(auctionID, userID uint64) int {
	room, ok := h.AuctionRooms[auctionID]
	if !ok {
		return 0
	}
	
	count := 0
	for conn := range room {
		if conn.UserID == userID {
			count++
		}
	}
	return count
}

// updateStats 更新統計資訊
func (h *Hub) updateStats(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			h.mutex.RLock()
			
			totalConnections := 0
			roomStats := make(map[uint64]int)
			
			for auctionID, room := range h.AuctionRooms {
				roomCount := len(room)
				roomStats[auctionID] = roomCount
				totalConnections += roomCount
			}
			
			h.Stats.TotalConnections = totalConnections
			h.Stats.AuctionRoomCount = len(h.AuctionRooms)
			h.Stats.AuctionRoomStats = roomStats
			h.Stats.LastUpdated = time.Now()
			
			h.mutex.RUnlock()
			
			h.Logger.Debug("Hub stats updated",
				zap.Int("total_connections", totalConnections),
				zap.Int("auction_rooms", len(h.AuctionRooms)),
			)
			
		case <-ctx.Done():
			return
		}
	}
}

// monitorDegradedLevel 監控降級等級
func (h *Hub) monitorDegradedLevel(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			level := h.calculateDegradedLevel()
			if level != h.Stats.DegradedLevel {
				oldLevel := h.Stats.DegradedLevel
				h.Stats.DegradedLevel = level
				
				// 更新降級管理器
				h.DegradationMgr.UpdateLevel(level)
				
				h.Logger.Info("Degraded level changed",
					zap.Int("old_level", oldLevel),
					zap.Int("new_level", level),
				)
				
				// 廣播降級等級變化給所有連接
				h.broadcastDegradedLevelChange(level)
			}
			
		case <-ctx.Done():
			return
		}
	}
}

// calculateDegradedLevel 計算降級等級
func (h *Hub) calculateDegradedLevel() int {
	h.mutex.RLock()
	totalConnections := h.Stats.TotalConnections
	h.mutex.RUnlock()
	
	// 簡單的降級邏輯（實際應該根據系統負載、錯誤率等）
	if totalConnections > 1000 {
		return 4 // 極限負載
	} else if totalConnections > 500 {
		return 3 // 高負載
	} else if totalConnections > 200 {
		return 2 // 中等負載
	} else if totalConnections > 100 {
		return 1 // 輕微負載
	}
	
	return 0 // 正常
}

// broadcastDegradedLevelChange 廣播降級等級變化
func (h *Hub) broadcastDegradedLevelChange(level int) {
	for auctionID := range h.AuctionRooms {
		message := Message{
			Type: MessageTypeState,
			Data: map[string]interface{}{
				"degraded_level": level,
			},
		}
		
		broadcast := &BroadcastMessage{
			AuctionID: auctionID,
			Message:   message,
		}
		
		select {
		case h.Broadcast <- broadcast:
		default:
			h.Logger.Warn("Broadcast channel full, dropping degraded level message")
		}
	}
}

// listenRedisPubSub 監聽 Redis 發布訂閱
func (h *Hub) listenRedisPubSub(ctx context.Context) {
	pubsub := h.Redis.Subscribe(ctx, "auction_events")
	defer pubsub.Close()
	
	ch := pubsub.Channel()
	
	for {
		select {
		case msg := <-ch:
			h.handleRedisMessage(msg)
		case <-ctx.Done():
			return
		}
	}
}

// handleRedisMessage 處理 Redis 訊息
func (h *Hub) handleRedisMessage(msg *redis.Message) {
	var event struct {
		AuctionID uint64      `json:"auction_id"`
		Type      string      `json:"type"`
		Data      interface{} `json:"data"`
	}
	
	if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
		h.Logger.Error("Failed to unmarshal Redis message", zap.Error(err))
		return
	}
	
	message := Message{
		Type: event.Type,
		Data: event.Data,
	}
	
	broadcast := &BroadcastMessage{
		AuctionID: event.AuctionID,
		Message:   message,
	}
	
	select {
	case h.Broadcast <- broadcast:
	default:
		h.Logger.Warn("Broadcast channel full, dropping Redis message")
	}
}

// closeAllConnections 關閉所有連接
func (h *Hub) closeAllConnections() {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	
	for _, room := range h.AuctionRooms {
		for conn := range room {
			close(conn.Send)
		}
	}
	
	h.AuctionRooms = make(map[uint64]map[*Connection]bool)
}

// BroadcastToAuction 向拍賣房間廣播訊息
func (h *Hub) BroadcastToAuction(auctionID uint64, msgType string, data interface{}) {
	message := Message{
		Type: msgType,
		Data: data,
	}
	
	broadcast := &BroadcastMessage{
		AuctionID: auctionID,
		Message:   message,
	}
	
	// 使用降級管理器控制消息佇列
	if !h.DegradationMgr.QueueMessage(broadcast) {
		h.Logger.Warn("Failed to queue message, degradation active",
			zap.Uint64("auction_id", auctionID),
			zap.String("message_type", msgType),
			zap.Int("degradation_level", h.Stats.DegradedLevel),
		)
		return
	}
	
	// 嘗試立即發送或加入廣播佇列
	select {
	case h.Broadcast <- broadcast:
	default:
		h.Logger.Warn("Broadcast channel full, message queued",
			zap.Uint64("auction_id", auctionID),
			zap.String("message_type", msgType),
		)
	}
}

// BroadcastToUser 向特定用戶廣播訊息
func (h *Hub) BroadcastToUser(auctionID, userID uint64, msgType string, data interface{}) {
	message := Message{
		Type: msgType,
		Data: data,
	}
	
	broadcast := &BroadcastMessage{
		AuctionID:   auctionID,
		Message:     message,
		TargetUsers: []uint64{userID},
	}
	
	select {
	case h.Broadcast <- broadcast:
	default:
		h.Logger.Warn("Broadcast channel full, dropping user message",
			zap.Uint64("auction_id", auctionID),
			zap.Uint64("user_id", userID),
		)
	}
}

// GetStats 取得 Hub 統計資訊
func (h *Hub) GetStats() *HubStats {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	
	// 複製統計資訊避免競態條件
	stats := &HubStats{
		TotalConnections: h.Stats.TotalConnections,
		AuctionRoomCount: h.Stats.AuctionRoomCount,
		DegradedLevel:    h.Stats.DegradedLevel,
		LastUpdated:      h.Stats.LastUpdated,
		AuctionRoomStats: make(map[uint64]int),
	}
	
	for k, v := range h.Stats.AuctionRoomStats {
		stats.AuctionRoomStats[k] = v
	}
	
	return stats
}

// cleanupDegradationLimiters 清理降級限制器
func (h *Hub) cleanupDegradationLimiters(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			h.DegradationMgr.CleanupRateLimiters()
		case <-ctx.Done():
			return
		}
	}
}