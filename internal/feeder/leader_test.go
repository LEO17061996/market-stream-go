package feeder

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/testutil"
)

func TestLeaderLockIsExclusiveAndReleasable(t *testing.T) {
	ctx := context.Background()
	b := bus.New(testutil.Redis(t))
	if ok, _ := b.HoldLeader(ctx, "a", time.Second); !ok {
		t.Fatal("A phải giành được khoá trống")
	}
	if ok, _ := b.HoldLeader(ctx, "b", time.Second); ok {
		t.Fatal("B không được giành khi A đang giữ")
	}
	if ok, _ := b.HoldLeader(ctx, "a", time.Second); !ok {
		t.Fatal("A phải gia hạn được khoá của chính mình")
	}
	_ = b.ReleaseLeader(ctx, "b") // B trả khoá không phải của mình: không có tác dụng
	if ok, _ := b.HoldLeader(ctx, "b", time.Second); ok {
		t.Fatal("B không được xoá khoá của A")
	}
	_ = b.ReleaseLeader(ctx, "a")
	if ok, _ := b.HoldLeader(ctx, "b", time.Second); !ok {
		t.Fatal("A trả khoá thì B phải tiếp quản được ngay")
	}
}

// Hai bộ sinh giá chạy chồng nhau (như lúc deploy): giá của mỗi mã phải là một chuỗi liền mạch,
// tức mọi tick đều do một bộ sinh ra; bộ thứ hai chỉ tiếp quản khi bộ đầu dừng.
func TestTwoFeedsNeverWriteAtTheSameTime(t *testing.T) {
	rdb := testutil.Redis(t)
	b := bus.New(rdb)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps := b.Subscribe(ctx)
	defer ps.Close()
	if _, err := ps.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	ch := ps.Channel()

	cfg := func(id string, seed uint64) Config {
		return Config{ID: id, Seed: seed, Interval: 20 * time.Millisecond, HistoryBars: 5, CandleFrame: time.Minute}
	}
	ctxA, stopA := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = Run(ctxA, b, cfg("a", 1), quiet) }()
	time.Sleep(200 * time.Millisecond)
	go func() { defer wg.Done(); _ = Run(ctx, b, cfg("b", 2), quiet) }()

	count := 0
	deadline := time.After(800 * time.Millisecond)
collect:
	for {
		select {
		case <-ch:
			count++
		case <-deadline:
			break collect
		}
	}
	if count == 0 {
		t.Fatal("phải có tick khi A giữ khoá")
	}
	if ok, _ := b.HoldLeader(ctx, "b", leaderTTL); ok {
		t.Fatal("B không được giữ khoá khi A còn chạy")
	}
	stopA() // A dừng êm, trả khoá
	time.Sleep(1500 * time.Millisecond)
	q, _ := b.Quotes(ctx)
	before := q["GC"].Time
	time.Sleep(300 * time.Millisecond)
	q, _ = b.Quotes(ctx)
	if q["GC"].Time <= before && q["CL"].Time <= before {
		t.Fatal("A dừng thì B phải tiếp quản và tiếp tục ghi giá")
	}
	cancel()
	wg.Wait()
}
