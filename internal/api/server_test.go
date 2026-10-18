package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LEO17061996/market-stream-go/internal/api"
	"github.com/LEO17061996/market-stream-go/internal/auth"
	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/market"
	"github.com/LEO17061996/market-stream-go/internal/stream"
	"github.com/LEO17061996/market-stream-go/internal/testutil"
)

type env struct {
	srv *httptest.Server
	bus *bus.Bus
	cli *http.Client
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rdb := testutil.Redis(t)
	st := testutil.Store(t)
	if err := st.SeedDemo(ctx, "demo", "123456", "Khách hàng Demo", 1); err != nil {
		t.Fatal(err)
	}
	b := bus.New(rdb)
	hub := stream.NewHub(log)
	go hub.Listen(ctx, b)
	// bảng giá tối thiểu để API tính ký quỹ
	_ = b.Write(ctx, bus.Update{
		Ticks:  []market.Tick{{Symbol: "GC", Price: 4000, Time: time.Now().UnixMilli()}},
		Quotes: []market.Quote{{Symbol: "GC", Price: 4000}, {Symbol: market.USDVND, Price: 26000}},
	})
	s := &api.Server{Bus: b, Hub: hub, Store: st, Sessions: auth.NewSessions(rdb), Log: log}
	srv := httptest.NewServer(s.Router())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &env{srv: srv, bus: b, cli: &http.Client{Jar: jar}}
}

func (e *env) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := e.cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestLoginSessionAndAccount(t *testing.T) {
	e := setup(t)
	if code, _ := e.do(t, "GET", "/api/me", nil); code != 401 {
		t.Fatalf("chưa đăng nhập phải 401, được %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/login", map[string]string{"username": "demo", "password": "sai"}); code != 401 {
		t.Fatalf("sai mật khẩu phải 401, được %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/login", map[string]string{"username": "demo", "password": "123456"}); code != 200 {
		t.Fatalf("đăng nhập đúng phải 200, được %d", code)
	}
	code, me := e.do(t, "GET", "/api/me", nil)
	if code != 200 || me["account"].(map[string]any)["availableVND"].(float64) <= 0 {
		t.Fatalf("/api/me sai: %d %v", code, me)
	}
	if code, _ := e.do(t, "POST", "/api/logout", map[string]string{}); code != 204 {
		t.Fatalf("đăng xuất phải 204, được %d", code)
	}
	if code, _ := e.do(t, "GET", "/api/me", nil); code != 401 {
		t.Fatal("đăng xuất xong phiên phải hết hiệu lực")
	}
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	e := setup(t)
	for i := 0; i < 10; i++ {
		e.do(t, "POST", "/api/login", map[string]string{"username": "demo", "password": "sai"})
	}
	code, body := e.do(t, "POST", "/api/login", map[string]string{"username": "demo", "password": "123456"})
	if code != 429 {
		t.Fatalf("lần thứ 11 phải bị chặn 429 kể cả đúng mật khẩu, được %d %v", code, body)
	}
}

func TestWritesRequireJSON(t *testing.T) {
	e := setup(t)
	res, err := e.cli.Post(e.srv.URL+"/api/login", "application/x-www-form-urlencoded", strings.NewReader("username=demo&password=123456"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("form gửi từ trang lạ phải bị chặn 415, được %d", res.StatusCode)
	}
}

func TestWithdrawalRules(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", "/api/login", map[string]string{"username": "demo", "password": "123456"})
	_, me := e.do(t, "GET", "/api/me", nil)
	avail := int64(me["account"].(map[string]any)["availableVND"].(float64))

	if code, _ := e.do(t, "POST", "/api/withdrawals", map[string]any{"amountVND": 50_000}); code != 400 {
		t.Fatalf("dưới 100.000 đ phải 400, được %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/withdrawals", map[string]any{"amountVND": avail + 1000}); code != 409 {
		t.Fatalf("rút quá khả dụng phải 409, được %d", code)
	}
	if code, _ := e.do(t, "POST", "/api/withdrawals", map[string]any{"amountVND": 1_000_000}); code != 201 {
		t.Fatalf("rút hợp lệ phải 201, được %d", code)
	}
	_, page := e.do(t, "GET", "/api/transactions?limit=5", nil)
	items := page["items"].([]any)
	first := items[0].(map[string]any)
	if len(items) != 5 || first["status"] != "pending" || first["amountVND"].(float64) != 1_000_000 {
		t.Fatalf("giao dịch mới nhất phải là yêu cầu vừa tạo: %v", first)
	}
	if page["nextBefore"].(float64) == 0 {
		t.Fatal("còn trang sau thì phải có nextBefore")
	}
}

func TestMarginUsesCachedPricesAndAccount(t *testing.T) {
	e := setup(t)
	code, body := e.do(t, "POST", "/api/margin", map[string]any{"symbol": "GC", "lots": 2})
	m := body["margin"].(map[string]any)
	if code != 200 || m["marginVND"].(float64) != 858_000_000 || body["maxLots"] != nil {
		t.Fatalf("ký quỹ 2 hợp đồng vàng sai: %d %v", code, body)
	}
	if code, _ := e.do(t, "POST", "/api/margin", map[string]any{"symbol": "USDVND", "lots": 1}); code != 400 {
		t.Fatal("tỷ giá không phải hợp đồng giao dịch, phải 400")
	}
	if code, _ := e.do(t, "POST", "/api/margin", map[string]any{"symbol": "GC", "lots": 1.5}); code != 400 {
		t.Fatal("số hợp đồng lẻ phải 400")
	}
	e.do(t, "POST", "/api/login", map[string]string{"username": "demo", "password": "123456"})
	_, body = e.do(t, "POST", "/api/margin", map[string]any{"symbol": "GC", "lots": 1})
	if body["maxLots"] == nil {
		t.Fatal("đã đăng nhập thì phải kèm số hợp đồng tối đa mở được")
	}
}

// Đầu cuối: bộ sinh giá ghi vào Redis → hub nghe pub/sub → trình duyệt nhận qua WebSocket.
func TestWebSocketReceivesTicksFromRedis(t *testing.T) {
	e := setup(t)
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var hello map[string]any
	if err := conn.ReadJSON(&hello); err != nil || hello["type"] != "hello" {
		t.Fatalf("tin đầu tiên phải là hello: %v %v", hello, err)
	}
	if err := conn.WriteJSON(map[string]any{"op": "subscribe", "symbols": []string{"CL"}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // chờ lệnh subscribe tới máy chủ

	ctx := context.Background()
	now := time.Now().UnixMilli()
	_ = e.bus.Write(ctx, bus.Update{Ticks: []market.Tick{{Symbol: "GC", Price: 4001, Time: now}}})
	_ = e.bus.Write(ctx, bus.Update{Ticks: []market.Tick{{Symbol: "CL", Price: 62.5, Time: now}}})

	var msg struct {
		Type string        `json:"type"`
		Data []market.Tick `json:"data"`
	}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "ticks" || len(msg.Data) != 1 || msg.Data[0].Symbol != "CL" {
		t.Fatalf("chỉ theo dõi CL thì không được nhận GC: %+v", msg)
	}

	if err := conn.WriteJSON(map[string]any{"op": "ping", "id": 7}); err != nil {
		t.Fatal(err)
	}
	var pong map[string]any
	if err := conn.ReadJSON(&pong); err != nil || pong["type"] != "pong" || pong["id"].(float64) != 7 {
		t.Fatalf("ping phải được trả pong cùng id: %v %v", pong, err)
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	e := setup(t)
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws"
	_, res, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"https://trang-la.example"}})
	if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("trang khác nguồn gốc phải bị từ chối 403, được %v", err)
	}
}

func TestQuotesAndCandles(t *testing.T) {
	e := setup(t)
	res, err := e.cli.Get(e.srv.URL + "/api/quotes")
	if err != nil {
		t.Fatal(err)
	}
	var quotes []market.Quote
	_ = json.NewDecoder(res.Body).Decode(&quotes)
	res.Body.Close()
	if len(quotes) != 2 || quotes[0].Symbol != "GC" {
		t.Fatalf("bảng giá phải xếp theo danh mục: %+v", quotes)
	}
	if code, _ := e.do(t, "GET", "/api/candles?symbol=XYZ", nil); code != 400 {
		t.Fatal("mã lạ phải 400")
	}
}
