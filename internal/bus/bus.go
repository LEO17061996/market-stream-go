// Package bus là lớp Redis dùng chung giữa bộ sinh giá và các máy chủ WebSocket:
// pub/sub để đẩy tick, hash để cache bảng giá, list để giữ lịch sử nến.
package bus

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/LEO17061996/market-stream-go/internal/market"
)

const (
	ChannelTicks  = "md:ticks"
	keyQuotes     = "md:quotes"     // hash: mã → Quote
	keyCandleCur  = "md:candle:cur" // hash: mã → nến đang mở
	keyCandlesPre = "md:candles:1m:"
	MaxCandles    = 600 // giữ 10 giờ nến 1 phút cho mỗi mã
)

type Bus struct{ rdb redis.UniversalClient }

func New(rdb redis.UniversalClient) *Bus { return &Bus{rdb: rdb} }

func (b *Bus) Client() redis.UniversalClient { return b.rdb }

type SymbolCandle struct {
	Symbol string
	Candle market.Candle
}

// Update là kết quả một bước của bộ sinh giá, ghi vào Redis trong một lần gọi mạng.
type Update struct {
	Ticks   []market.Tick
	Quotes  []market.Quote
	Closed  []SymbolCandle
	Current []SymbolCandle
}

func (b *Bus) Write(ctx context.Context, u Update) error {
	if len(u.Ticks) == 0 {
		return nil
	}
	pipe := b.rdb.TxPipeline()
	quotes := make([]any, 0, len(u.Quotes)*2)
	for _, q := range u.Quotes {
		quotes = append(quotes, q.Symbol, mustJSON(q))
	}
	if len(quotes) > 0 { // HSET không có trường là lỗi cú pháp, làm Redis huỷ cả MULTI, kéo theo PUBLISH
		pipe.HSet(ctx, keyQuotes, quotes...)
	}
	for _, c := range u.Closed {
		key := keyCandlesPre + c.Symbol
		pipe.RPush(ctx, key, mustJSON(c.Candle))
		pipe.LTrim(ctx, key, -MaxCandles, -1)
	}
	cur := make([]any, 0, len(u.Current)*2)
	for _, c := range u.Current {
		cur = append(cur, c.Symbol, mustJSON(c.Candle))
	}
	if len(cur) > 0 {
		pipe.HSet(ctx, keyCandleCur, cur...)
	}
	pipe.Publish(ctx, ChannelTicks, mustJSON(u.Ticks))
	_, err := pipe.Exec(ctx)
	return err
}

// Quotes đọc toàn bộ bảng giá từ cache, không chạm cơ sở dữ liệu.
func (b *Bus) Quotes(ctx context.Context) (map[string]market.Quote, error) {
	raw, err := b.rdb.HGetAll(ctx, keyQuotes).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]market.Quote, len(raw))
	for sym, v := range raw {
		var q market.Quote
		if json.Unmarshal([]byte(v), &q) == nil {
			out[sym] = q
		}
	}
	return out, nil
}

func (b *Bus) CurrentCandles(ctx context.Context) (map[string]market.Candle, error) {
	raw, err := b.rdb.HGetAll(ctx, keyCandleCur).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]market.Candle, len(raw))
	for sym, v := range raw {
		var c market.Candle
		if json.Unmarshal([]byte(v), &c) == nil {
			out[sym] = c
		}
	}
	return out, nil
}

// Candles trả về tối đa limit nến gần nhất, kể cả nến đang mở.
func (b *Bus) Candles(ctx context.Context, symbol string, limit int) ([]market.Candle, error) {
	if limit <= 0 || limit > MaxCandles {
		limit = MaxCandles
	}
	raw, err := b.rdb.LRange(ctx, keyCandlesPre+symbol, int64(-limit), -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]market.Candle, 0, len(raw)+1)
	for _, v := range raw {
		var c market.Candle
		if json.Unmarshal([]byte(v), &c) == nil {
			out = append(out, c)
		}
	}
	cur, err := b.rdb.HGet(ctx, keyCandleCur, symbol).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	var c market.Candle
	if cur != "" && json.Unmarshal([]byte(cur), &c) == nil && (len(out) == 0 || c.Time > out[len(out)-1].Time) {
		out = append(out, c)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (b *Bus) HasHistory(ctx context.Context, symbol string) (bool, error) {
	n, err := b.rdb.LLen(ctx, keyCandlesPre+symbol).Result()
	return n > 0, err
}

// SeedHistory ghi nến lịch sử và bảng giá ban đầu (chỉ gọi khi Redis còn trống).
func (b *Bus) SeedHistory(ctx context.Context, hist map[string][]market.Candle, quotes []market.Quote) error {
	pipe := b.rdb.TxPipeline()
	for sym, candles := range hist {
		key := keyCandlesPre + sym
		vals := make([]any, len(candles))
		for i, c := range candles {
			vals[i] = mustJSON(c)
		}
		pipe.Del(ctx, key)
		if len(vals) > 0 {
			pipe.RPush(ctx, key, vals...)
			pipe.LTrim(ctx, key, -MaxCandles, -1)
		}
	}
	pipe.Del(ctx, keyCandleCur)
	for _, q := range quotes {
		pipe.HSet(ctx, keyQuotes, q.Symbol, mustJSON(q))
	}
	_, err := pipe.Exec(ctx)
	return err
}

const keyLeader = "md:feed:leader"

var renewLeader = redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("PEXPIRE", KEYS[1], ARGV[2]) else return 0 end`)
var releaseLeader = redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`)

// HoldLeader giành hoặc gia hạn quyền ghi giá. Chỉ một bộ sinh giá được ghi tại một thời điểm,
// kể cả khi hai bản chạy chồng nhau lúc deploy; bản kia chờ và tiếp quản khi khoá hết hạn.
func (b *Bus) HoldLeader(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	ok, err := b.rdb.SetNX(ctx, keyLeader, id, ttl).Result()
	if err != nil || ok {
		return ok, err
	}
	n, err := renewLeader.Run(ctx, b.rdb, []string{keyLeader}, id, ttl.Milliseconds()).Int()
	return n == 1, err
}

// ReleaseLeader trả khoá khi dừng êm để bản khác tiếp quản ngay, không phải chờ hết hạn.
func (b *Bus) ReleaseLeader(ctx context.Context, id string) error {
	return releaseLeader.Run(ctx, b.rdb, []string{keyLeader}, id).Err()
}

func (b *Bus) Subscribe(ctx context.Context) *redis.PubSub {
	return b.rdb.Subscribe(ctx, ChannelTicks)
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // chỉ mã hoá struct của chính mình, không thể lỗi
	}
	return string(data)
}
