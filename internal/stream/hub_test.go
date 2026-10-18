package stream

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/LEO17061996/market-stream-go/internal/market"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func fakeClient(h *Hub, buffer int) *Client {
	c := &Client{hub: h, send: make(chan []byte, buffer), all: true}
	h.add(c)
	return c
}

func decode(t *testing.T, msg []byte) []market.Tick {
	t.Helper()
	var m outMessage
	if err := json.Unmarshal(msg, &m); err != nil || m.Type != "ticks" {
		t.Fatalf("tin không hợp lệ: %s", msg)
	}
	return m.Data
}

func TestBroadcastFiltersBySubscription(t *testing.T) {
	h := NewHub(quiet)
	all := fakeClient(h, 8)
	gold := fakeClient(h, 8)
	gold.subscribe([]string{"GC", "KHONG-CO"})

	h.Broadcast([]market.Tick{{Symbol: "GC", Price: 4000, Time: 1}, {Symbol: "CL", Price: 62, Time: 1}})

	if got := decode(t, <-all.send); len(got) != 2 {
		t.Fatalf("client theo dõi tất cả phải nhận 2 tick, nhận %d", len(got))
	}
	got := decode(t, <-gold.send)
	if len(got) != 1 || got[0].Symbol != "GC" {
		t.Fatalf("client chỉ theo dõi vàng nhận sai: %+v", got)
	}

	h.Broadcast([]market.Tick{{Symbol: "CL", Price: 63, Time: 2}})
	select {
	case msg := <-gold.send:
		t.Fatalf("lô không có mã đã theo dõi thì không được gửi: %s", msg)
	default:
	}
}

func TestSlowClientIsDroppedWithoutBlockingOthers(t *testing.T) {
	h := NewHub(quiet)
	slow := fakeClient(h, 1)
	fast := fakeClient(h, 64)
	for i := 0; i < 5; i++ {
		h.Broadcast([]market.Tick{{Symbol: "GC", Price: float64(4000 + i), Time: int64(i)}})
		<-fast.send // client nhanh đọc kịp từng lô
	}
	if h.Stats().Clients != 1 || h.Stats().DroppedSlow != 1 {
		t.Fatalf("client chậm phải bị ngắt đúng một lần: %+v", h.Stats())
	}
	if _, open := <-slow.send; !open {
		return
	}
	// còn 1 tin trong bộ đệm; đọc tiếp thì kênh phải đã đóng
	if _, open := <-slow.send; open {
		t.Fatal("hàng đợi của client chậm phải bị đóng")
	}
}

func TestTrySendAfterRemoveDoesNotPanic(t *testing.T) {
	h := NewHub(quiet)
	c := fakeClient(h, 1)
	h.remove(c)
	h.trySend(c, []byte(`{}`)) // trước đây có thể panic vì gửi vào kênh đã đóng
	h.remove(c)                // gỡ hai lần cũng không panic
}
