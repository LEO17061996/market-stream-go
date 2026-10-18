// Package feeder chạy bộ sinh giá: mỗi bước sinh tick, cập nhật bảng giá và nến, ghi vào Redis.
// Chỉ chạy MỘT bộ sinh giá; các máy chủ WebSocket nhận giá qua Redis nên chạy bao nhiêu bản cũng được.
package feeder

import (
	"context"
	"log/slog"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/market"
)

type Config struct {
	Seed         uint64
	Interval     time.Duration
	HistoryBars  int
	CandleFrame  time.Duration
	Now          func() time.Time
}

func Run(ctx context.Context, b *bus.Bus, cfg Config, log *slog.Logger) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	f := market.NewFeed(cfg.Seed)
	book := market.NewQuoteBook()
	agg := market.NewAggregator(cfg.CandleFrame)

	if err := restoreOrSeed(ctx, b, cfg, f, book, agg, log); err != nil {
		return err
	}

	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			u := step(f, book, agg, cfg.Now())
			if err := b.Write(ctx, u); err != nil && ctx.Err() == nil {
				log.Warn("ghi Redis lỗi, bỏ bước này", "err", err)
			}
		}
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
