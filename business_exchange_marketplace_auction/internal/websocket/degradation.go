package websocket

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// DegradationManager handles system load-based message throttling and connection limits
// implementing a 5-level degradation system (0-4) with adaptive rate limiting
type DegradationManager struct {
	level           int
	messageQueue    chan *BroadcastMessage
	priorityQueue   chan *BroadcastMessage
	rateLimiter     map[uint64]*ConnectionRateLimiter
	rateLimiterMux  sync.RWMutex
	logger          *zap.Logger
}

// ConnectionRateLimiter tracks per-user message rates within time windows
// for implementing degradation-aware throttling policies
type ConnectionRateLimiter struct {
	UserID       uint64
	LastMessage  time.Time
	MessageCount int
	WindowStart  time.Time
}

// NewDegradationManager 創建降級管理器
func NewDegradationManager(logger *zap.Logger) *DegradationManager {
	return &DegradationManager{
		level:         0,
		messageQueue:  make(chan *BroadcastMessage, 10000),
		priorityQueue: make(chan *BroadcastMessage, 1000),
		rateLimiter:   make(map[uint64]*ConnectionRateLimiter),
		logger:        logger,
	}
}

// UpdateLevel 更新降級等級
func (dm *DegradationManager) UpdateLevel(level int) {
	if dm.level != level {
		dm.logger.Info("Degradation level updated",
			zap.Int("old_level", dm.level),
			zap.Int("new_level", level),
		)
		dm.level = level
		dm.adjustRateLimits(level)
	}
}

// ShouldThrottleMessage determines if a user's message should be throttled
// based on current degradation level, message frequency, and rate limits
func (dm *DegradationManager) ShouldThrottleMessage(userID uint64, msgType string) bool {
	dm.rateLimiterMux.Lock()
	defer dm.rateLimiterMux.Unlock()
	
	now := time.Now()
	limiter, exists := dm.rateLimiter[userID]
	
	if !exists {
		dm.rateLimiter[userID] = &ConnectionRateLimiter{
			UserID:      userID,
			LastMessage: now,
			MessageCount: 1,
			WindowStart:  now,
		}
		return false
	}
	
	// 重置計數窗口（1分鐘）
	if now.Sub(limiter.WindowStart) > time.Minute {
		limiter.MessageCount = 1
		limiter.WindowStart = now
		limiter.LastMessage = now
		return false
	}
	
	// 根據降級等級調整限制
	maxMessages := dm.getMaxMessagesPerWindow()
	minInterval := dm.getMinMessageInterval()
	
	// 檢查頻率限制
	if now.Sub(limiter.LastMessage) < minInterval {
		return true
	}
	
	// 檢查消息數量限制
	if limiter.MessageCount >= maxMessages {
		return true
	}
	
	limiter.MessageCount++
	limiter.LastMessage = now
	return false
}

// IsHighPriorityMessage identifies critical messages that bypass normal throttling
// including auction extensions, closures, and error notifications
func (dm *DegradationManager) IsHighPriorityMessage(msgType string) bool {
	switch msgType {
	case MessageTypeExtended, MessageTypeClosed, MessageTypeError:
		return true
	default:
		return false
	}
}

// QueueMessage adds messages to either priority or normal queues
// with different capacity limits based on message importance
func (dm *DegradationManager) QueueMessage(msg *BroadcastMessage) bool {
	if dm.IsHighPriorityMessage(msg.Message.Type) {
		select {
		case dm.priorityQueue <- msg:
			return true
		default:
			return false // 優先級佇列滿了，丟棄消息
		}
	}
	
	select {
	case dm.messageQueue <- msg:
		return true
	default:
		return false // 一般佇列滿了，丟棄消息
	}
}

// GetNextMessage 取得下一個要處理的消息
func (dm *DegradationManager) GetNextMessage() *BroadcastMessage {
	// 優先處理高優先級消息
	select {
	case msg := <-dm.priorityQueue:
		return msg
	default:
	}
	
	// 處理一般消息
	select {
	case msg := <-dm.messageQueue:
		return msg
	default:
		return nil
	}
}

// adjustRateLimits 根據降級等級調整速率限制
func (dm *DegradationManager) adjustRateLimits(level int) {
	// 清理舊的限制器
	dm.rateLimiterMux.Lock()
	defer dm.rateLimiterMux.Unlock()
	
	// 在高負載情況下，更激進地清理限制器
	if level >= 3 {
		dm.rateLimiter = make(map[uint64]*ConnectionRateLimiter)
	}
}

// getMaxMessagesPerWindow returns the maximum messages allowed per minute
// based on current degradation level: 60/30/15/5/1 messages for levels 0-4
func (dm *DegradationManager) getMaxMessagesPerWindow() int {
	switch dm.level {
	case 0: // 正常
		return 60  // 60 messages/min
	case 1: // 輕微負載
		return 30  // 30 messages/min
	case 2: // 中等負載
		return 15  // 15 messages/min
	case 3: // 高負載
		return 5   // 5 messages/min
	case 4: // 極限負載
		return 1   // 1 message/min
	default:
		return 60
	}
}

// getMinMessageInterval returns the minimum time between messages
// based on degradation level: 100ms/500ms/2s/5s/30s for levels 0-4
func (dm *DegradationManager) getMinMessageInterval() time.Duration {
	switch dm.level {
	case 0: // 正常
		return 100 * time.Millisecond
	case 1: // 輕微負載
		return 500 * time.Millisecond
	case 2: // 中等負載
		return 2 * time.Second
	case 3: // 高負載
		return 5 * time.Second
	case 4: // 極限負載
		return 30 * time.Second
	default:
		return 100 * time.Millisecond
	}
}

// CleanupRateLimiters 清理過期的限制器
func (dm *DegradationManager) CleanupRateLimiters() {
	dm.rateLimiterMux.Lock()
	defer dm.rateLimiterMux.Unlock()
	
	now := time.Now()
	for userID, limiter := range dm.rateLimiter {
		// 清理5分鐘內沒有活動的限制器
		if now.Sub(limiter.LastMessage) > 5*time.Minute {
			delete(dm.rateLimiter, userID)
		}
	}
}