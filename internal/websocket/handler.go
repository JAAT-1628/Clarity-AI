package websocket

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/infrastructure/queue"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WebSocketConnection struct {
	ID             string
	AnalysisID     string
	UserID         int64
	Conn           *websocket.Conn
	Send           chan []byte
	StreamingQueue *queue.StreamingRedisQueue
	subscription   *models.StreamSubscription
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
}

type WebSocketHub struct {
	connections    map[string]*WebSocketConnection
	register       chan *WebSocketConnection
	unregister     chan *WebSocketConnection
	streamingQueue *queue.StreamingRedisQueue
	mu             sync.RWMutex
}

func NewWebSocketHub(streamingQueue *queue.StreamingRedisQueue) *WebSocketHub {
	return &WebSocketHub{
		connections:    make(map[string]*WebSocketConnection),
		register:       make(chan *WebSocketConnection),
		unregister:     make(chan *WebSocketConnection),
		streamingQueue: streamingQueue,
	}
}

func (h *WebSocketHub) Run() {
	log.Println("🚀 Starting WebSocket Hub...")

	go h.cleanupRoutine()

	for {
		select {
		case conn := <-h.register:
			h.registerConnection(conn)
		case conn := <-h.unregister:
			h.unregisterConnection(conn)
		}
	}
}

func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/stream/video/")
	analysisID := strings.Split(urlPath, "/")[0]

	if analysisID == "" {
		http.Error(w, "analysis_id is required", http.StatusBadRequest)
		return
	}

	if len(analysisID) != 36 {
		http.Error(w, "Invalid analysis_id format", http.StatusBadRequest)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == authHeader {
		http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
		return
	}

	// TODO: Validate JWT token here
	// userID, err := validateJWTToken(token)
	// if err != nil {
	//     http.Error(w, "Invalid token", http.StatusUnauthorized)
	//     return
	// }

	userID := int64(5)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed: %v", err)
		return
	}

	log.Printf("🔗 New WebSocket connection for analysis %s (user %d)", analysisID, userID)

	ctx, cancel := context.WithCancel(context.Background())
	wsConn := &WebSocketConnection{
		ID:             fmt.Sprintf("ws_%s_%d_%d", analysisID, userID, time.Now().Unix()),
		AnalysisID:     analysisID,
		UserID:         userID,
		Conn:           conn,
		Send:           make(chan []byte, 1000),
		StreamingQueue: h.streamingQueue,
		ctx:            ctx,
		cancel:         cancel,
	}

	h.register <- wsConn
	go wsConn.writePump(h)
	go wsConn.readPump(h)
}

func (h *WebSocketHub) registerConnection(conn *WebSocketConnection) {
	h.mu.Lock()
	h.connections[conn.ID] = conn
	h.mu.Unlock()

	go conn.subscribeToStream()

	welcomeMsg := map[string]interface{}{
		"type":          "connection_established",
		"connection_id": conn.ID,
		"analysis_id":   conn.AnalysisID,
		"timestamp":     time.Now(),
		"message":       "Connected to real-time analysis stream",
	}

	if data, err := json.Marshal(welcomeMsg); err == nil {
		select {
		case conn.Send <- data:
		default:
		}
	}
}

func (h *WebSocketHub) unregisterConnection(conn *WebSocketConnection) {
	h.mu.Lock()
	if _, exists := h.connections[conn.ID]; exists {
		delete(h.connections, conn.ID)
		close(conn.Send)
		conn.cancel()
		log.Printf("🔌 Unregistered WebSocket connection %s (remaining: %d)", conn.ID, len(h.connections))
	}
	h.mu.Unlock()
}

func (h *WebSocketHub) GetConnectionCount() int {
	h.mu.RLock()
	count := len(h.connections)
	h.mu.RUnlock()
	return count
}

func (h *WebSocketHub) GetConnectionsForAnalysis(analysisID string) []*WebSocketConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var connections []*WebSocketConnection
	for _, conn := range h.connections {
		if conn.AnalysisID == analysisID {
			connections = append(connections, conn)
		}
	}
	return connections
}

func (h *WebSocketHub) cleanupRoutine() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		h.mu.Lock()
		for id, conn := range h.connections {
			conn.mu.Lock()
			select {
			case <-conn.ctx.Done():
				delete(h.connections, id)
				close(conn.Send)
			default:
				//
			}
			conn.mu.Unlock()
		}
		h.mu.Unlock()
	}
}

func (conn *WebSocketConnection) subscribeToStream() {
	log.Printf("🔄 Starting subscription for analysis %s", conn.AnalysisID)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Panic in WebSocket subscription for %s: %v", conn.ID, r)
		}
		log.Printf("🔄 Subscription ended for WebSocket %s", conn.ID)
	}()

	var subscription *models.StreamSubscription
	var err error
	for attempts := 0; attempts < 5; attempts++ {
		log.Printf("🔄 Subscription attempt %d for WebSocket %s", attempts+1, conn.ID)
		subscription, err = conn.StreamingQueue.SubscribeToAnalysisStream(conn.ctx, conn.AnalysisID, conn.UserID)
		if err == nil {
			break
		}
		log.Printf("❌ Subscription attempt %d failed for WebSocket %s: %v", attempts+1, conn.ID, err)
		if attempts < 4 {
			backoff := time.Duration(attempts+1) * time.Second
			log.Printf("⏰ Retrying subscription in %v", backoff)
			time.Sleep(backoff)
		}
	}
	if err != nil {
		log.Printf("❌ Failed to subscribe after 5 attempts for WebSocket %s: %v", conn.ID, err)
		errorMsg := map[string]interface{}{
			"type":    "subscription_failed",
			"message": "Failed to subscribe after multiple attempts",
			"error":   err.Error(),
		}
		if data, err2 := json.Marshal(errorMsg); err2 == nil {
			select {
			case conn.Send <- data:
			default:
			}
		}
		return
	}
	conn.subscription = subscription
	log.Printf("📡 WebSocket %s successfully subscribed to analysis stream %s", conn.ID, conn.AnalysisID)

	successMsg := map[string]interface{}{
		"type":            "subscription_success",
		"message":         "Successfully subscribed to analysis stream",
		"analysis_id":     conn.AnalysisID,
		"subscription_id": subscription.ID,
	}
	if data, err := json.Marshal(successMsg); err == nil {
		select {
		case conn.Send <- data:
		default:
		}
	}

	expectedChunk := 1
	chunkBuf := make(map[int][]map[string]interface{})
	chunkDone := make(map[int]bool)

	send := func(m map[string]interface{}) {
		data, err := json.Marshal(m)
		if err != nil {
			log.Printf("❌ Failed to marshal WebSocket message: %v", err)
			return
		}
		select {
		case conn.Send <- data:
		case <-time.After(10 * time.Second):
			log.Printf("⚠️ WebSocket %s send timeout, dropping message", conn.ID)
		}
	}

	flushContiguous := func() {
		for {
			if !chunkDone[expectedChunk] {
				break
			}

			if queue, ok := chunkBuf[expectedChunk]; ok {
				for _, m := range queue {
					send(m)
				}
				delete(chunkBuf, expectedChunk)
			}
			expectedChunk++
		}
	}

	getEventType := func(m map[string]interface{}) string {
		if s, ok := m["event_type"].(string); ok {
			return s
		}
		if s, ok := m["eventType"].(string); ok {
			return s
		}
		return ""
	}

	getChunkNum := func(m map[string]interface{}) int {
		v, ok := m["chunk_number"]
		if !ok || v == nil {
			return 0
		}
		switch t := v.(type) {
		case int:
			return t
		case int32:
			return int(t)
		case int64:
			return int(t)
		case float64:
			return int(t)
		case json.Number:
			if n, err := strconv.Atoi(string(t)); err == nil {
				return n
			}
		}
		return 0
	}

	for {
		select {
		case <-conn.ctx.Done():
			log.Printf("🔄 WebSocket context cancelled for %s", conn.ID)
			return

		case msg, ok := <-subscription.Channel:
			if !ok {
				log.Printf("⚠️ Subscription channel closed for WebSocket %s", conn.ID)
				channelClosedMsg := map[string]interface{}{
					"type":      "channel_closed",
					"message":   "Redis subscription channel closed",
					"timestamp": time.Now(),
				}
				send(channelClosedMsg)
				return
			}

			wsMessage := conn.convertStreamMessageToWebSocket(msg)

			cn := getChunkNum(wsMessage)
			if cn <= 0 {
				send(wsMessage)
				continue
			}

			chunkBuf[cn] = append(chunkBuf[cn], wsMessage)

			if getEventType(wsMessage) == string(models.EventChunkCompleted) {
				chunkDone[cn] = true
			}

			flushContiguous()
		}
	}
}

func (conn *WebSocketConnection) convertStreamMessageToWebSocket(msg *models.StreamMessage) map[string]interface{} {
	wsMsg := map[string]interface{}{
		"type":         "stream_update",
		"event_type":   string(msg.EventType),
		"analysis_id":  msg.AnalysisID,
		"chunk_id":     msg.ChunkID,
		"chunk_number": msg.ChunkNumber,
		"timestamp":    msg.Timestamp,
		"sequence":     msg.Sequence,
	}

	if msg.Progress != nil {
		wsMsg["progress"] = map[string]interface{}{
			"chunk_progress":      msg.Progress.ChunkProgress,
			"overall_progress":    msg.Progress.OverallProgress,
			"current_stage":       msg.Progress.CurrentStage,
			"completed_chunks":    msg.Progress.CompletedChunks,
			"total_chunks":        msg.Progress.TotalChunks,
			"estimated_time_left": msg.Progress.EstimatedTimeLeft,
		}
	}

	if msg.Error != nil {
		wsMsg["error"] = map[string]interface{}{
			"code":    msg.Error.Code,
			"message": msg.Error.Message,
			"stage":   msg.Error.Stage,
		}
	}

	if msg.Data != nil {
		wsMsg["data"] = msg.Data
	}

	return wsMsg
}

func (conn *WebSocketConnection) readPump(hub *WebSocketHub) {
	defer func() {
		conn.Conn.SetCloseHandler(func(code int, text string) error {
			hub.unregister <- conn
			return nil
		})
	}()
	conn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.Conn.SetPongHandler(func(string) error {
		conn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ WebSocket read error: %v", err)
			}
			break
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			conn.handleIncomingMessage(msg)
		}
	}
}

func (conn *WebSocketConnection) writePump(hub *WebSocketHub) {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		conn.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-conn.Send:
			conn.mu.Lock()
			conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = conn.Conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutdown"),
					time.Now().Add(2*time.Second),
				)
				conn.mu.Unlock()
				return
			}
			if err := conn.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("❌ WebSocket write error for %s: %v", conn.ID, err)
				conn.mu.Unlock()
				return
			}
			conn.mu.Unlock()

		case <-ticker.C:
			conn.mu.Lock()
			conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if err := conn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("❌ WebSocket ping failed for %s: %v", conn.ID, err)
				conn.mu.Unlock()
				return
			}

			heartbeat := map[string]interface{}{
				"type":          "heartbeat",
				"timestamp":     time.Now(),
				"connection_id": conn.ID,
				"analysis_id":   conn.AnalysisID,
			}
			if data, err := json.Marshal(heartbeat); err == nil {
				select {
				case conn.Send <- data:
					log.Printf("💓 Sent heartbeat to WebSocket %s", conn.ID)
				default:
					log.Printf("⚠️ Failed to send heartbeat to WebSocket %s (channel full)", conn.ID)
				}
			}

			conn.mu.Unlock()
		}
	}
}

func (conn *WebSocketConnection) handleIncomingMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "ping":
		response := map[string]interface{}{
			"type":      "pong",
			"timestamp": time.Now(),
		}
		if data, err := json.Marshal(response); err == nil {
			select {
			case conn.Send <- data:
			default:
			}
		}

	case "request_history":
		go conn.sendHistoricalMessages()

	case "heartbeat":
		log.Printf("Heartbeat from WebSocket %s", conn.ID)
	}
}

func (conn *WebSocketConnection) sendHistoricalMessages() {
	history, err := conn.StreamingQueue.GetStreamHistory(conn.ctx, conn.AnalysisID, 20)
	if err != nil {
		log.Printf("❌ Failed to get history for WebSocket %s: %v", conn.ID, err)
		return
	}

	historyResponse := map[string]interface{}{
		"type":        "history_response",
		"analysis_id": conn.AnalysisID,
		"messages":    make([]map[string]interface{}, len(history)),
		"timestamp":   time.Now(),
	}

	for i, msg := range history {
		historyResponse["messages"].([]map[string]interface{})[i] = conn.convertStreamMessageToWebSocket(msg)
	}

	if data, err := json.Marshal(historyResponse); err == nil {
		select {
		case conn.Send <- data:
			log.Printf("📚 Sent history to WebSocket %s (%d messages)", conn.ID, len(history))
		default:
			log.Printf("⚠️ Failed to send history to WebSocket %s (channel full)", conn.ID)
		}
	}
}

type WebSocketHandler struct {
	hub *WebSocketHub
}

func NewWebSocketHandler(streamingQueue *queue.StreamingRedisQueue) *WebSocketHandler {
	hub := NewWebSocketHub(streamingQueue)
	go hub.Run()

	return &WebSocketHandler{
		hub: hub,
	}
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.hub.HandleWebSocket(w, r)
}

func (h *WebSocketHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"active_connections": h.hub.GetConnectionCount(),
		"timestamp":          time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *WebSocketHandler) GetAnalysisConnections(w http.ResponseWriter, r *http.Request) {
	analysisID := r.URL.Query().Get("analysis_id")
	if analysisID == "" {
		http.Error(w, "analysis_id is required", http.StatusBadRequest)
		return
	}

	connections := h.hub.GetConnectionsForAnalysis(analysisID)

	response := map[string]interface{}{
		"analysis_id":        analysisID,
		"active_connections": len(connections),
		"timestamp":          time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ws://localhost:8080/stream/video/96c92612-ab1e-4646-8695-3926f1ff75ea
// {
// 	"user_id": 5,
// 	"video_url": "https://www.youtube.com/watch?v=VkGQFFl66X4",
// 	"video_title": "Advanced Golang: Channels, Context and Interfaces Explained",
// 	"ai_provider": "AI_PROVIDER_FREE",
// 	"options": {
// 	  "chunk_duration_minutes": 2,
// 	  "enable_real_time_streaming": true,
// 	  "language": "en"
// 	}
//   }
