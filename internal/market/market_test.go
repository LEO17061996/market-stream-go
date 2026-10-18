package market

import (
	"testing"
	"time"
)

func TestRoundToTick(t *testing.T) {
	cases := []struct{ in, tick, want float64 }{
		{4010.04, 0.1, 4010.0},
		{4010.06, 0.1, 4010.1},
		{421.63, 0.25, 421.75},
		{5.02031, 0.0005, 5.0205},
		{26310.4, 1, 26310},
	}
	for _, c := range cases {
		if got := RoundToTick(c.in, c.tick); got != c.want {
			t.Errorf("RoundToTick(%v, %v) = %v, muốn %v", c.in, c.tick, got, c.want)
		}
	}
}

func TestFeedIsDeterministicAndStaysOnTickGrid(t *testing.T) {
	a, b := NewFeed(42), NewFeed(42)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 2000; i++ {
		now = now.Add(250 * time.Millisecond)
		ta, tb := a.Step(now), b.Step(now)
		if len(ta) != len(tb) {
			t.Fatalf("bước %d: cùng seed mà khác số tick", i)
		}
		for j, tick := range ta {
			if tick != tb[j] {
				t.Fatalf("bước %d: cùng seed mà khác tick %v / %v", i, tick, tb[j])
			}
			in, _ := Find(tick.Symbol)
			if tick.Price <= 0 || RoundToTick(tick.Price, in.TickSize) != tick.Price {
				t.Fatalf("giá %v của %s không nằm trên bước giá %v", tick.Price, tick.Symbol, in.TickSize)
			}
		}
	}
}

func TestBackfillEndsAtCurrentPrice(t *testing.T) {
	f := NewFeed(7)
	end := time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)
	hist := f.Backfill(end, 120, time.Minute)
	gc := hist["GC"]
	if len(gc) != 120 {
		t.Fatalf("muốn 120 nến, có %d", len(gc))
	}
	for i := 1; i < len(gc); i++ {
		if gc[i].Time-gc[i-1].Time != 60 {
			t.Fatalf("nến %d không cách đều 60 giây", i)
		}
		if gc[i].Open != gc[i-1].Close {
			t.Fatalf("nến %d mở cửa %v khác đóng cửa nến trước %v", i, gc[i].Open, gc[i-1].Close)
		}
		if gc[i].High < gc[i].Low || gc[i].High < gc[i].Open || gc[i].Low > gc[i].Close {
			t.Fatalf("nến %d sai OHLC: %+v", i, gc[i])
		}
	}
	if f.Price("GC") != gc[len(gc)-1].Close {
		t.Fatal("giá hiện tại phải nối tiếp nến cuối")
	}
}

func TestAggregatorOpensClosesAndIgnoresLateTicks(t *testing.T) {
	a := NewAggregator(time.Minute)
	base := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC).UnixMilli()
	add := func(ms int64, p float64) (*Candle, Candle, bool) {
		return a.Add(Tick{Symbol: "GC", Price: p, Time: base + ms})
	}
	add(1_000, 100)
	add(20_000, 103)
	add(40_000, 99)
	closed, cur, _ := add(61_000, 101)
	if closed == nil {
		t.Fatal("tick ở phút mới phải đóng nến cũ")
	}
	want := Candle{Time: base / 1000, Open: 100, High: 103, Low: 99, Close: 99}
	if *closed != want {
		t.Fatalf("nến đóng = %+v, muốn %+v", *closed, want)
	}
	if cur.Time != base/1000+60 || cur.Open != 99 || cur.Close != 101 || cur.High != 101 || cur.Low != 99 {
		t.Fatalf("nến mới sai: %+v", cur)
	}
	if _, _, ok := add(59_000, 50); ok {
		t.Fatal("tick đến muộn của phút đã đóng phải bị bỏ qua")
	}
}

func TestQuoteBookResetsAtVietnamMidnight(t *testing.T) {
	b := NewQuoteBook()
	// 23:59 và 00:01 giờ Việt Nam ngày hôm sau
	beforeMidnight := time.Date(2026, 10, 19, 16, 59, 0, 0, time.UTC).UnixMilli()
	afterMidnight := time.Date(2026, 10, 19, 17, 1, 0, 0, time.UTC).UnixMilli()
	b.Apply(Tick{Symbol: "GC", Price: 4000, Time: beforeMidnight - 60_000})
	q := b.Apply(Tick{Symbol: "GC", Price: 4040, Time: beforeMidnight})
	if q.Change != 40 || q.ChangePct != 1 {
		t.Fatalf("thay đổi trong ngày sai: %+v", q)
	}
	q = b.Apply(Tick{Symbol: "GC", Price: 4050, Time: afterMidnight})
	if q.Open != 4050 || q.Change != 0 || q.High != 4050 || q.Low != 4050 {
		t.Fatalf("sang ngày mới phải mở cửa lại: %+v", q)
	}
}

func TestCalcMargin(t *testing.T) {
	gc, _ := Find("GC")
	m := CalcMargin(gc, 2, 4000, 26000)
	if m.NotionalUSD != 800_000 || m.MarginUSD != 33_000 || m.MarginVND != 858_000_000 {
		t.Fatalf("ký quỹ vàng sai: %+v", m)
	}
	zc, _ := Find("ZC") // giá tính bằng cent
	m = CalcMargin(zc, 1, 420, 26000)
	if m.NotionalUSD != 21_000 {
		t.Fatalf("giá trị hợp đồng ngô phải quy cent sang USD: %+v", m)
	}
}
