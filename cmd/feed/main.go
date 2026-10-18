// feed: bộ sinh giá giả lập. Chỉ chạy MỘT bản; ghi bảng giá, nến và phát tick qua Redis.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/config"
	"github.com/LEO17061996/market-stream-go/internal/feeder"
)

func main() {
	log := config.Logger()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb, err := config.Redis()
	if err != nil {
		log.Error("REDIS_URL sai", "err", err)
		os.Exit(1)
	}
	log.Info("bộ sinh giá chạy", "interval", config.FeedInterval())
	cfg := feeder.Config{Seed: uint64(time.Now().UnixNano()), Interval: config.FeedInterval(), HistoryBars: 300, CandleFrame: time.Minute}
	if err := feeder.Run(ctx, bus.New(rdb), cfg, log); err != nil {
		log.Error("bộ sinh giá dừng", "err", err)
		os.Exit(1)
	}
}
