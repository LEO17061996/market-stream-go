// Package stream đẩy giá tới trình duyệt qua WebSocket.
//
// Mỗi máy chủ có một Hub, nghe Redis pub/sub một lần rồi phát cho mọi client đang nối.
// Client đọc chậm (hàng đợi đầy) bị ngắt ngay để không làm chậm những client khác.
package stream

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/market"
)

const sendBuffer = 256

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	log     *slog.Logger

	batches   atomic.Int64
	dropped   atomic.Int64
	lastTick  atomic.Int64 // unix ms của tick mới nhất nhận từ Redis
	lagMillis atomic.Int64 // độ trễ từ lúc sinh tick tới lúc hub nhận
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{clients: map[*Client]struct{}{}, log: log}
}

type Stats struct {
	Clients       int   `json:"clients"`
	Batches       int64 `json:"batches"`
	DroppedSlow   int64 `json:"droppedSlowClients"`
	LastTickAt    int64 `json:"lastTickAt"`
	FeedToHubLag  int64 `json:"feedToHubLagMs"`
}

func (h *Hub) Stats() Stats {
	h.mu.RLock()
	n := len(h.clients)
	h.mu.RUnlock()
	return Stats{Clients: n, Batches: h.batches.Load(), DroppedSlow: h.dropped.Load(), LastTickAt: h.lastTick.Load(), FeedToHubLag: h.lagMillis.Load()}
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

// trySend gửi một tin riêng cho client (hello, pong). Giữ khoá đọc để hàng đợi không bị đóng giữa chừng.
func (h *Hub) trySend(c *Client, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if _, ok := h.clients[c]; !ok {
		return
	}
	select {
	case c.send <- msg:
	default:
	}
}

// remove gỡ client và đóng hàng đợi gửi đúng một lần.
func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

type outMessage struct {
	Type string        `json:"type"`
	Data []market.Tick `json:"data"`
}

// Broadcast gửi một lô tick. Client theo dõi tất cả dùng chung một bản JSON đã mã hoá sẵn.
func (h *Hub) Broadcast(ticks []market.Tick) {
	if len(ticks) == 0 {
		return
	}
	h.batches.Add(1)
	now := time.Now().UnixMilli()
	h.lastTick.Store(ticks[len(ticks)-1].Time)
	h.lagMillis.Store(now - ticks[len(ticks)-1].Time)

	all, _ := json.Marshal(outMessage{Type: "ticks", Data: ticks})
	var slow []*Client

	h.mu.RLock()
	for c := range h.clients {
		msg := all
		if !c.wantsAll() {
			picked := c.filter(ticks)
			if len(picked) == 0 {
				continue
			}
			msg, _ = json.Marshal(outMessage{Type: "ticks", Data: picked})
		}
		select {
		case c.send <- msg:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range slow {
		h.dropped.Add(1)
		h.log.Warn("ngắt client đọc chậm", "remote", c.remote)
		h.remove(c)
	}
}

// Listen nghe Redis và phát lại cho client; go-redis tự nối lại khi Redis rớt.
func (h *Hub) Listen(ctx context.Context, b *bus.Bus) {
	ps := b.Subscribe(ctx)
	defer ps.Close()
	ch := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var ticks []market.Tick
			if err := json.Unmarshal([]byte(msg.Payload), &ticks); err != nil {
				h.log.Warn("bỏ gói tick hỏng", "err", err)
				continue
			}
			h.Broadcast(ticks)
		}
	}
}
