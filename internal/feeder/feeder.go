// Package feeder chạy bộ sinh giá: mỗi bước sinh tick, cập nhật bảng giá và nến, ghi vào Redis.
// Tại một thời điểm chỉ MỘT bộ sinh giá được ghi (khoá trong Redis); các máy chủ WebSocket nhận giá
// qua Redis nên chạy bao nhiêu bản cũng được.
package feeder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/market"
)

const leaderTTL = 3 * time.Second

type Config struct {
	Seed        uint64
	Interval    time.Duration
	HistoryBars int
	CandleFrame time.Duration
	Now         func() time.Time
	ID          string // định danh bộ sinh giá khi giành khoá; trống thì tự sinh
}

// Run chờ tới lượt giữ khoá rồi mới sinh giá; mất khoá (bản khác chiếm khi mạng chập chờn) thì dừng ghi và chờ lại.
func Run(ctx context.Context, b *bus.Bus, cfg Config, log *slog.Logger) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.ID == "" {
		cfg.ID = newID()
	}
	defer b.ReleaseLeader(context.Background(), cfg.ID)
	waiting := false
	for ctx.Err() == nil {
		ok, err := b.HoldLeader(ctx, cfg.ID, leaderTTL)
		if err != nil || !ok {
			if !waiting {
				log.Info("bộ sinh giá khác đang ghi, chờ tới lượt", "id", cfg.ID)
				waiting = true
			}
			sleep(ctx, time.Second)
			continue
		}
		waiting = false
		log.Info("giữ quyền ghi giá", "id", cfg.ID)
		if err := lead(ctx, b, cfg, log); err != nil {
			return err
		}
	}
	return nil
}

// lead sinh giá cho tới khi ctx kết thúc hoặc mất khoá.
func lead(ctx context.Context, b *bus.Bus, cfg Config, log *slog.Logger) error {
	f := market.NewFeed(cfg.Seed)
	book := market.NewQuoteBook()
	agg := market.NewAggregator(cfg.CandleFrame)
	if err := restoreOrSeed(ctx, b, cfg, f, book, agg, log); err != nil {
		return err
	}

	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	renew := time.NewTicker(leaderTTL / 3)
	defer renew.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-renew.C:
			if ok, err := b.HoldLeader(ctx, cfg.ID, leaderTTL); !ok && ctx.Err() == nil {
				log.Warn("mất quyền ghi giá, dừng ghi", "err", err)
				return nil
			}
		case <-t.C:
			u := step(f, book, agg, cfg.Now())
			if err := b.Write(ctx, u); err != nil && ctx.Err() == nil {
				log.Warn("ghi Redis lỗi, bỏ bước này", "err", err)
			}
		}
	}
}

func newID() string {
	host, _ := os.Hostname()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return host + "-" + hex.EncodeToString(buf)
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func step(f *market.Feed, book *market.QuoteBook, agg *market.Aggregator, now time.Time) bus.Update {
	ticks := f.Step(now)
	u := bus.Update{Ticks: ticks}
	for _, tk := range ticks {
		u.Quotes = append(u.Quotes, book.Apply(tk))
		closed, cur, ok := agg.Add(tk)
		if !ok {
			continue
		}
		if closed != nil {
			u.Closed = append(u.Closed, bus.SymbolCandle{Symbol: tk.Symbol, Candle: *closed})
		}
		u.Current = append(u.Current, bus.SymbolCandle{Symbol: tk.Symbol, Candle: cur})
	}
	return u
}

// restoreOrSeed: khởi động lại thì nối tiếp giá cũ trong Redis; Redis trống thì sinh lịch sử nến.
func restoreOrSeed(ctx context.Context, b *bus.Bus, cfg Config, f *market.Feed, book *market.QuoteBook, agg *market.Aggregator, log *slog.Logger) error {
	has, err := b.HasHistory(ctx, "GC")
	if err != nil {
		return err
	}
	if has {
		quotes, err := b.Quotes(ctx)
		if err != nil {
			return err
		}
		for _, q := range quotes {
			f.SetPrice(q.Symbol, q.Price)
			book.Seed(q)
		}
		cur, err := b.CurrentCandles(ctx)
		if err != nil {
			return err
		}
		for sym, c := range cur {
			agg.Seed(sym, c)
		}
		log.Info("nối tiếp giá từ Redis", "symbols", len(quotes))
		return nil
	}

	now := cfg.Now()
	hist := f.Backfill(now, cfg.HistoryBars, cfg.CandleFrame)
	quotes := make([]market.Quote, 0, len(hist))
	for _, in := range market.Instruments {
		quotes = append(quotes, dayQuote(in.Symbol, hist[in.Symbol], f.Price(in.Symbol), now))
	}
	for _, q := range quotes {
		book.Seed(q)
	}
	log.Info("Redis trống, sinh lịch sử nến", "bars", cfg.HistoryBars)
	return b.SeedHistory(ctx, hist, quotes)
}

// dayQuote dựng dòng bảng giá ban đầu từ các nến lịch sử thuộc ngày hôm nay (giờ Việt Nam).
func dayQuote(symbol string, candles []market.Candle, price float64, now time.Time) market.Quote {
	tmp := market.NewQuoteBook()
	for _, c := range candles {
		for _, p := range []float64{c.Open, c.High, c.Low, c.Close} {
			tmp.Apply(market.Tick{Symbol: symbol, Price: p, Time: c.Time * 1000})
		}
	}
	return tmp.Apply(market.Tick{Symbol: symbol, Price: price, Time: now.UnixMilli()})
}
