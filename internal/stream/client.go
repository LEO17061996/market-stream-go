package stream

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LEO17061996/market-stream-go/internal/market"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second // nhỏ hơn timeout của Nginx/Render để kết nối không bị cắt khi rảnh
	maxMessage = 4096
	maxSymbols = 50
)

// Upgrader mặc định chỉ nhận kết nối cùng nguồn gốc (chặn trang lạ mở WebSocket bằng cookie của người dùng).
var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	remote string

	mu      sync.RWMutex
	all     bool
	symbols map[string]bool
}

func (c *Client) wantsAll() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.all
}

func (c *Client) filter(ticks []market.Tick) []market.Tick {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []market.Tick
	for _, t := range ticks {
		if c.symbols[t.Symbol] {
			out = append(out, t)
		}
	}
	return out
}

// inMessage là lệnh từ trình duyệt:
//
//	{"op":"subscribe","symbols":["*"]}   theo dõi tất cả
//	{"op":"subscribe","symbols":["GC"]}  chỉ một vài mã
//	{"op":"ping","id":7}                 đo độ trễ khứ hồi
type inMessage struct {
	Op      string   `json:"op"`
	Symbols []string `json:"symbols"`
	ID      int64    `json:"id"`
}

// ServeWS nâng cấp kết nối HTTP lên WebSocket và gắn vào hub.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade đã tự trả lỗi HTTP
	}
	c := &Client{hub: h, conn: conn, send: make(chan []byte, sendBuffer), remote: r.RemoteAddr, all: true}
	h.add(c)
	hello, _ := json.Marshal(map[string]any{"type": "hello", "serverTime": time.Now().UnixMilli()})
	h.trySend(c, hello)
	go c.writePump()
	go c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c)
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessage)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var m inMessage
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Op {
		case "subscribe":
			c.subscribe(m.Symbols)
		case "ping":
			pong, _ := json.Marshal(map[string]any{"type": "pong", "id": m.ID, "serverTime": time.Now().UnixMilli()})
			c.hub.trySend(c, pong)
		}
	}
}

func (c *Client) subscribe(symbols []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.all = false
	c.symbols = map[string]bool{}
	for i, s := range symbols {
		if i >= maxSymbols {
			break
		}
		if s == "*" {
			c.all = true
			continue
		}
		if _, ok := market.Find(s); ok {
			c.symbols[s] = true
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "client chậm"))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
