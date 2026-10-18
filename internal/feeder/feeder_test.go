package feeder

import (
	"context"
	"log/slog"
	"io"
	"testing"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/market"
	"github.com/LEO17061996/market-stream-go/internal/testutil"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestSeedThenStepWritesQuotesAndCandles(t *testing.T) {
	ctx := context.Background()
	b := bus.New(testutil.Redis(t))
	now := time.Date(2026, 10, 19, 2, 30, 10, 0, time.UTC)
	cfg := Config{Seed: 1, Interval: time.Second, HistoryBars: 120, CandleFrame: time.Minute, Now: func() time.Time { return now }}

	f := market.NewFeed(cfg.Seed)
	book := market.NewQuoteBook()
	agg := market.NewAggregator(cfg.CandleFrame)
	if err := restoreOrSeed(ctx, b, cfg, f, book, agg, quiet); err != nil {
		t.Fatal(err)
	}
	hist, err := b.Candles(ctx, "GC", 600)
	if err != nil || len(hist) != 120 {
		t.Fatalf("muốn 120 nến lịch sử, được %d (%v)", len(hist), err)
	}
	quotes, _ := b.Quotes(ctx)
	if len(quotes) != len(market.Instruments) || quotes["GC"].Price != hist[len(hist)-1].Close {
		t.Fatalf("bảng giá ban đầu phải nối tiếp nến cuối: %+v", quotes["GC"])
	}

	// Chạy 3 phút: phải có nến đóng mới nối vào lịch sử, nến đang mở nằm cuối danh sách.
	for i := 0; i < 720; i++ {
		now = now.Add(250 * time.Millisecond)
		if err := b.Write(ctx, step(f, book, agg, now)); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := b.Candles(ctx, "GC", 600)
	if len(after) < 123 {
		t.Fatalf("sau 3 phút phải có thêm nến, có %d", len(after))
	}
	last := after[len(after)-1]
	if last.Time != market.BucketStart(now.UnixMilli(), time.Minute) {
		t.Fatalf("nến cuối phải là nến của phút hiện tại: %+v", last)
	}
	q, _ := b.Quotes(ctx)
	if q["GC"].Price != f.Price("GC") {
		t.Fatal("bảng giá trong Redis phải bằng giá hiện tại của bộ sinh giá")
	}
}

func TestRestartContinuesFromRedis(t *testing.T) {
	ctx := context.Background()
	rdb := testutil.Redis(t)
	b := bus.New(rdb)
	now := time.Date(2026, 10, 19, 2, 30, 0, 0, time.UTC)
	cfg := Config{Seed: 1, Interval: time.Second, HistoryBars: 30, CandleFrame: time.Minute, Now: func() time.Time { return now }}
	f1 := market.NewFeed(1)
	if err := restoreOrSeed(ctx, b, cfg, f1, market.NewQuoteBook(), market.NewAggregator(time.Minute), quiet); err != nil {
		t.Fatal(err)
	}
	// Bộ sinh giá thứ hai (khởi động lại, seed khác) phải lấy lại giá cũ chứ không về giá khởi điểm.
	f2 := market.NewFeed(99)
	if err := restoreOrSeed(ctx, b, cfg, f2, market.NewQuoteBook(), market.NewAggregator(time.Minute), quiet); err != nil {
		t.Fatal(err)
	}
	if f2.Price("GC") != f1.Price("GC") {
		t.Fatalf("khởi động lại phải nối tiếp giá %v, đang là %v", f1.Price("GC"), f2.Price("GC"))
	}
	n, _ := b.Candles(ctx, "GC", 600)
	if len(n) != 30 {
		t.Fatalf("khởi động lại không được sinh lại lịch sử, có %d nến", len(n))
	}
}
