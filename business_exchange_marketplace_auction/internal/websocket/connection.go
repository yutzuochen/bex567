package websocket

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"auction_service/internal/models"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 在生產環境中應該檢查來源
		return true
	},
}

// Connection WebSocket 連接
type Connection struct {
	ID         string
	AuctionID  uint64
	UserID     uint64
	Conn       *websocket.Conn
	Send       chan []byte
	Hub        *Hub
	Logger     *zap.Logger
	LastPong   time.Time
	LastEventID uint64
	DegradedLevel int
}

// Message WebSocket 訊息格式
type Message struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data,omitempty"`
	EventID   uint64      `json:"event_id,omitempty"`
	ServerTime time.Time  `json:"server_time"`
}

// ClientMessage 客戶端訊息
type ClientMessage struct {
	Type        string  `json:"type"`
	Amount      float64 `json:"amount,omitempty"`
	ClientSeq   int64   `json:"client_seq,omitempty"`
	LastEventID uint64  `json:"last_event_id,omitempty"`
}

const (
	// WebSocket 訊息類型
	MessageTypeHello        = "hello"
	MessageTypeState        = "state"
	MessageTypeExtended     = "extended"
	MessageTypeBidAccepted  = "bid_accepted"
	MessageTypeBidRejected  = "bid_rejected"
	MessageTypeClosed       = "closed"
	MessageTypeResumeOK     = "resume_ok"
	MessageTypePing         = "ping"
	MessageTypePong         = "pong"
	MessageTypeError        = "error"
	
	// 客戶端訊息類型
	ClientMessageTypePlaceBid = "place_bid"
	ClientMessageTypeResume   = "resume"
	ClientMessageTypePong     = "pong"
)

const (
	// Time constants
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// NewConnection 創建新的 WebSocket 連接
func NewConnection(hub *Hub, conn *websocket.Conn, auctionID, userID uint64, logger *zap.Logger) *Connection {
	return &Connection{
		ID:        generateConnectionID(),
		AuctionID: auctionID,
		UserID:    userID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Hub:       hub,
		Logger:    logger,
		LastPong:  time.Now(),
		DegradedLevel: 0,
	}
}

// Start 啟動連接處理
func (c *Connection) Start() {
	c.Hub.Register <- c
	
	// 發送歡迎訊息
	c.sendHelloMessage()
	
	go c.writePump()
	go c.readPump()
}

// readPump 處理讀取訊息
func (c *Connection) readPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.LastPong = time.Now()
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Logger.Error("WebSocket read error", 
					zap.String("connection_id", c.ID),
					zap.Uint64("user_id", c.UserID),
					zap.Error(err),
				)
			}
			break
		}

		var clientMsg ClientMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			c.Logger.Error("Invalid client message", 
				zap.String("connection_id", c.ID),
				zap.Error(err),
			)
			continue
		}

		c.handleClientMessage(&clientMsg)
	}
}

// writePump 處理發送訊息
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量發送排隊的訊息
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleClientMessage 處理客戶端訊息
func (c *Connection) handleClientMessage(msg *ClientMessage) {
	switch msg.Type {
	case ClientMessageTypePlaceBid:
		c.handlePlaceBid(msg)
	case ClientMessageTypeResume:
		c.handleResume(msg)
	case ClientMessageTypePong:
		c.LastPong = time.Now()
	default:
		c.Logger.Warn("Unknown client message type", 
			zap.String("type", msg.Type),
			zap.String("connection_id", c.ID),
		)
	}
}

// handlePlaceBid 處理出價請求
func (c *Connection) handlePlaceBid(msg *ClientMessage) {
	// 透過 HTTP 處理器處理出價邏輯
	// 這裡我們會調用現有的 BidHandler.PlaceBid 邏輯
	// 但透過 WebSocket 返回結果
	
	c.Logger.Info("WebSocket bid request",
		zap.String("connection_id", c.ID),
		zap.Uint64("user_id", c.UserID),
		zap.Uint64("auction_id", c.AuctionID),
		zap.Float64("amount", msg.Amount),
	)
	
	// 發送出價確認（這裡先簡化處理）
	response := Message{
		Type: MessageTypeBidAccepted,
		Data: map[string]interface{}{
			"amount": msg.Amount,
			"accepted": true,
		},
		ServerTime: time.Now(),
	}
	
	c.SendMessage(response)
}

// handleResume 處理斷線恢復
func (c *Connection) handleResume(msg *ClientMessage) {
	c.Logger.Info("WebSocket resume request",
		zap.String("connection_id", c.ID),
		zap.Uint64("user_id", c.UserID),
		zap.Uint64("last_event_id", msg.LastEventID),
	)
	
	// 取得遺漏的事件
	missedEvents := c.getMissedEvents(msg.LastEventID)
	
	response := Message{
		Type: MessageTypeResumeOK,
		Data: map[string]interface{}{
			"missed": missedEvents,
		},
		ServerTime: time.Now(),
	}
	
	c.SendMessage(response)
}

// sendHelloMessage 發送歡迎訊息
func (c *Connection) sendHelloMessage() {
	// 取得拍賣資訊和用戶狀態
	auction, alias := c.getAuctionInfo()
	
	response := Message{
		Type: MessageTypeHello,
		Data: map[string]interface{}{
			"status_code":     auction.StatusCode,
			"end_at":          auction.EndAt,
			"extended_until":  auction.ExtendedUntil,
			"alias_label":     alias,
			"can_bid":         c.canBid(),
			"degraded_level":  c.DegradedLevel,
		},
		ServerTime: time.Now(),
	}
	
	c.SendMessage(response)
}

// SendMessage 發送訊息給客戶端
func (c *Connection) SendMessage(msg Message) {
	msg.ServerTime = time.Now()
	data, err := json.Marshal(msg)
	if err != nil {
		c.Logger.Error("Failed to marshal message", 
			zap.String("connection_id", c.ID),
			zap.Error(err),
		)
		return
	}
	
	select {
	case c.Send <- data:
	default:
		c.Logger.Warn("Send channel full, closing connection", 
			zap.String("connection_id", c.ID),
		)
		close(c.Send)
	}
}

// Close 關閉連接
func (c *Connection) Close() {
	close(c.Send)
}

// getAuctionInfo 取得拍賣資訊
func (c *Connection) getAuctionInfo() (*models.Auction, string) {
	var auction models.Auction
	if err := c.Hub.DB.First(&auction, c.AuctionID).Error; err != nil {
		return &auction, ""
	}
	
	// 取得別名
	var alias models.AuctionBidderAlias
	if err := c.Hub.DB.Where("auction_id = ? AND bidder_id = ?", 
		c.AuctionID, c.UserID).First(&alias).Error; err == nil {
		return &auction, alias.AliasLabel
	}
	
	return &auction, ""
}

// canBid 檢查是否可以出價
func (c *Connection) canBid() bool {
	// 檢查黑名單
	var blacklist models.UserBlacklist
	if err := c.Hub.DB.Where("user_id = ? AND is_active = ?", 
		c.UserID, true).First(&blacklist).Error; err == nil {
		return false
	}
	
	// 檢查拍賣狀態
	var auction models.Auction
	if err := c.Hub.DB.First(&auction, c.AuctionID).Error; err != nil {
		return false
	}
	
	return auction.IsActive()
}

// getMissedEvents 取得遺漏的事件
func (c *Connection) getMissedEvents(lastEventID uint64) []models.AuctionEvent {
	var events []models.AuctionEvent
	c.Hub.DB.Where("auction_id = ? AND event_id > ?", 
		c.AuctionID, lastEventID).
		Order("event_id ASC").
		Limit(500).
		Find(&events)
	
	// 更新用戶的事件偏移量
	offset := &models.AuctionStreamOffset{
		AuctionID:   c.AuctionID,
		UserID:      c.UserID,
		LastEventID: lastEventID,
	}
	c.Hub.DB.Save(offset)
	
	return events
}

// generateConnectionID 生成連接ID
func generateConnectionID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}