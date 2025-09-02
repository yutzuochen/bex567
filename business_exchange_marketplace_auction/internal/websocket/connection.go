package websocket

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"auction_service/internal/models"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		userAgent := r.Header.Get("User-Agent")
		host := r.Header.Get("Host")
		log.Printf("[WS] CheckOrigin - Origin=%s, URL=%s, Host=%s, UserAgent=%s",
			origin, r.URL.String(), host, userAgent)
		log.Printf("[WS] CheckOrigin - Method=%s, Headers=%v", r.Method, r.Header)

		// Allow all origins for debugging
		log.Printf("[WS] CheckOrigin - ALLOWING connection")
		return true
	},
}

// Connection WebSocket 連接
type Connection struct {
	ID            string
	AuctionID     uint64
	UserID        uint64
	Conn          *websocket.Conn
	Send          chan []byte
	Hub           *Hub
	Logger        *zap.Logger
	LastPong      time.Time
	LastEventID   uint64
	DegradedLevel int
}

// Message WebSocket 訊息格式
type Message struct {
	Type       string      `json:"type"`
	Data       interface{} `json:"data,omitempty"`
	EventID    uint64      `json:"event_id,omitempty"`
	ServerTime time.Time   `json:"server_time"`
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
	MessageTypeHello       = "hello"
	MessageTypeState       = "state"
	MessageTypeExtended    = "extended"
	MessageTypeBidAccepted = "bid_accepted"
	MessageTypeBidRejected = "bid_rejected"
	MessageTypeClosed      = "closed"
	MessageTypeResumeOK    = "resume_ok"
	MessageTypePing        = "ping"
	MessageTypePong        = "pong"
	MessageTypeError       = "error"

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
		ID:            generateConnectionID(),
		AuctionID:     auctionID,
		UserID:        userID,
		Conn:          conn,
		Send:          make(chan []byte, 256),
		Hub:           hub,
		Logger:        logger,
		LastPong:      time.Now(),
		DegradedLevel: 0,
	}
}

// Start 啟動連接處理
func (c *Connection) Start() {
	c.Logger.Debug("Starting connection",
		zap.String("connection_id", c.ID),
		zap.Uint64("auction_id", c.AuctionID),
		zap.Uint64("user_id", c.UserID),
	)

	c.Hub.Register <- c

	c.Logger.Debug("Connection registered, starting pumps first",
		zap.String("connection_id", c.ID),
	)

	// Start pumps BEFORE sending messages to avoid race condition
	go c.writePump()
	go c.readPump()

	c.Logger.Debug("Pumps started, sending hello message",
		zap.String("connection_id", c.ID),
	)

	// 發送歡迎訊息 (after pumps are running)
	c.sendHelloMessage()
}

// readPump 處理讀取訊息
func (c *Connection) readPump() {
	c.Logger.Debug("Starting readPump",
		zap.String("connection_id", c.ID),
	)

	defer func() {
		c.Logger.Debug("readPump finished, unregistering connection",
			zap.String("connection_id", c.ID),
		)
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
			c.Logger.Debug("ReadMessage error in readPump",
				zap.String("connection_id", c.ID),
				zap.Error(err),
				zap.Bool("is_unexpected", websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure)),
			)
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
			"amount":   msg.Amount,
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
	c.Logger.Debug("Starting sendHelloMessage",
		zap.String("connection_id", c.ID),
		zap.Uint64("auction_id", c.AuctionID),
		zap.Uint64("user_id", c.UserID),
	)

	// 取得拍賣資訊和用戶狀態
	auction, alias := c.getAuctionInfo()

	// 如果拍賣不存在，發送錯誤訊息並關閉連接
	if auction == nil {
		c.Logger.Error("Auction not found, closing connection",
			zap.String("connection_id", c.ID),
			zap.Uint64("auction_id", c.AuctionID),
		)
		errorResponse := Message{
			Type: MessageTypeError,
			Data: map[string]interface{}{
				"code":    "auction_not_found",
				"message": "Auction not found",
			},
			ServerTime: time.Now(),
		}
		c.SendMessage(errorResponse)
		c.Close()
		return
	}

	c.Logger.Debug("Auction found successfully",
		zap.String("connection_id", c.ID),
		zap.Uint64("auction_id", c.AuctionID),
		zap.String("status_code", auction.StatusCode),
		zap.String("alias", alias),
	)

	// 準備回應資料
	helloData := map[string]interface{}{
		"status_code":    auction.StatusCode,
		"end_at":         auction.EndAt,
		"extended_until": auction.ExtendedUntil,
		"can_bid":        c.canBid(),
		"degraded_level": c.DegradedLevel,
		"has_bid":        alias != "", // 用戶是否已經出過價
	}

	// 只有在有別名時才添加
	if alias != "" {
		helloData["alias_label"] = alias
	}

	response := Message{
		Type:       MessageTypeHello,
		Data:       helloData,
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

	// Use a select with a default case to avoid blocking
	// and handle the case where the channel might be closed
	select {
	case c.Send <- data:
		// Message sent successfully
	default:
		// Channel is full or closed, unregister the connection
		c.Logger.Warn("Send channel full or closed, unregistering connection",
			zap.String("connection_id", c.ID),
		)
		// Don't close the channel here, let the hub handle it
		c.Hub.Unregister <- c
	}
}

// Close 關閉連接
func (c *Connection) Close() {
	// Don't close the channel directly, let the hub handle it
	c.Hub.Unregister <- c
}

// getAuctionInfo 取得拍賣資訊
func (c *Connection) getAuctionInfo() (*models.Auction, string) {
	var auction models.Auction
	if err := c.Hub.DB.First(&auction, c.AuctionID).Error; err != nil {
		c.Logger.Error("Failed to get auction info",
			zap.Uint64("auction_id", c.AuctionID),
			zap.Error(err),
		)
		return nil, ""
	}

	// 取得別名 (如果用戶已經出價過)
	var alias models.AuctionBidderAlias
	if err := c.Hub.DB.Where("auction_id = ? AND bidder_id = ?",
		c.AuctionID, c.UserID).First(&alias).Error; err == nil {
		return &auction, alias.AliasLabel
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.Logger.Error("Error fetching bidder alias",
			zap.Uint64("auction_id", c.AuctionID),
			zap.Uint64("bidder_id", c.UserID),
			zap.Error(err),
		)
	}
	// 如果沒有別名 (用戶尚未出價)，返回空字符串是正常的

	return &auction, ""
}

// canBid 檢查是否可以出價
func (c *Connection) canBid() bool {
	// 檢查黑名單 - 如果找到記錄則表示被封鎖，返回 false
	var blacklist models.UserBlacklist
	if err := c.Hub.DB.Where("user_id = ? AND is_active = ?",
		c.UserID, true).First(&blacklist).Error; err == nil {
		// 找到黑名單記錄，用戶被封鎖
		c.Logger.Debug("User is blacklisted",
			zap.Uint64("user_id", c.UserID),
		)
		return false
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.Logger.Error("Error checking blacklist",
			zap.Uint64("user_id", c.UserID),
			zap.Error(err),
		)
		return false
	}
	// 沒有找到黑名單記錄 (err == gorm.ErrRecordNotFound)，用戶未被封鎖，繼續檢查其他條件

	// 檢查拍賣狀態
	var auction models.Auction
	if err := c.Hub.DB.First(&auction, c.AuctionID).Error; err != nil {
		c.Logger.Error("Error checking auction status",
			zap.Uint64("auction_id", c.AuctionID),
			zap.Error(err),
		)
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
