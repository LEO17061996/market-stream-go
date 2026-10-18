// server: REST API + WebSocket + giao diện. Chạy được nhiều bản sau Nginx, mọi bản nhận giá qua Redis pub/sub.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/api"
	"github.com/LEO17061996/market-stream-go/internal/auth"
	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/config"
	"github.com/LEO17061996/market-stream-go/internal/feeder"
	"github.com/LEO17061996/market-stream-go/internal/store"
	"github.com/LEO17061996/market-stream-go/internal/stream"
	"github.com/LEO17061996/market-stream-go/web"
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
	st, err := store.Open(ctx, os.Getenv("MYSQL_DSN"))
	if err != nil {
		log.Error("không nối được cơ sở dữ liệu", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		log.Error("migrate lỗi", "err", err)
		os.Exit(1)
	}
	if err := st.SeedDemo(ctx, "demo", "123456", "Khách hàng Demo", 2026); err != nil {
		log.Error("tạo dữ liệu demo lỗi", "err", err)
		os.Exit(1)
	}

	b := bus.New(rdb)
	hub := stream.NewHub(log)
	go hub.Listen(ctx, b)

	// Bản chạy một mình (Render): bộ sinh giá chạy chung tiến trình. Docker Compose thì tách riêng cmd/feed.
	if os.Getenv("FEED_EMBEDDED") == "1" {
		cfg := feeder.Config{Seed: uint64(time.Now().UnixNano()), Interval: config.FeedInterval(), HistoryBars: 300, CandleFrame: time.Minute}
		go func() {
			if err := feeder.Run(ctx, b, cfg, log); err != nil {
				log.Error("bộ sinh giá dừng", "err", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:              ":" + config.Env("PORT", "8080"),
		Handler:           (&api.Server{Bus: b, Hub: hub, Store: st, Sessions: auth.NewSessions(rdb), Static: web.FS(), Log: log}).Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Info("máy chủ chạy", "addr", srv.Addr, "db", st.Dialect(), "feedEmbedded", os.Getenv("FEED_EMBEDDED") == "1")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("máy chủ dừng", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
