package market

import (
	"math"
	"math/rand/v2"
	"time"
)

// Tick là một lần đổi giá. Gửi qua Redis và WebSocket nên tên trường ngắn.
type Tick struct {
	Symbol string  `json:"s"`
	Price  float64 `json:"p"`
	Time   int64   `json:"t"` // unix mili giây, lúc bộ sinh giá tạo tick
}

// Feed sinh giá theo chuyển động Brown hình học, làm tròn theo bước giá của từng hợp đồng.
type Feed struct {
	rng    *rand.Rand
	prices map[string]float64
	last   time.Time
}

func NewFeed(seed uint64) *Feed {
	f := &Feed{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), prices: map[string]float64{}}
	for _, in := range Instruments {
		f.prices[in.Symbol] = in.startPrice
	}
	return f
}

// SetPrice đặt giá hiện tại (dùng khi khởi động lại để giá nối tiếp giá cũ trong Redis).
func (f *Feed) SetPrice(symbol string, price float64) {
	if _, ok := f.prices[symbol]; ok && price > 0 {
		f.prices[symbol] = price
	}
}

func (f *Feed) Price(symbol string) float64 { return f.prices[symbol] }

// Step tiến giá tới thời điểm now. Mỗi bước chỉ một phần các mã có tick, giống thị trường thật
// (không phải mã nào cũng khớp lệnh mỗi 250 ms).
func (f *Feed) Step(now time.Time) []Tick {
	dt := 0.25
	if !f.last.IsZero() {
		dt = math.Max(now.Sub(f.last).Seconds(), 0.001)
	}
	f.last = now
	ticks := make([]Tick, 0, len(Instruments))
	for _, in := range Instruments {
		if f.rng.Float64() > 0.55 {
			continue
		}
		next := f.walk(in, f.prices[in.Symbol], dt*4) // gom biến động của các bước bị bỏ qua
		if next == f.prices[in.Symbol] {
			continue
		}
		f.prices[in.Symbol] = next
		ticks = append(ticks, Tick{Symbol: in.Symbol, Price: next, Time: now.UnixMilli()})
	}
	return ticks
}

func (f *Feed) walk(in Instrument, price, dt float64) float64 {
	z := f.rng.NormFloat64()
	next := price * math.Exp(in.sigma*math.Sqrt(dt)*z-0.5*in.sigma*in.sigma*dt)
	return RoundToTick(next, in.TickSize)
}

// Backfill sinh n nến lịch sử kết thúc tại end cho từng mã, rồi đặt giá hiện tại bằng giá đóng cửa nến cuối.
func (f *Feed) Backfill(end time.Time, n int, tf time.Duration) map[string][]Candle {
	out := map[string][]Candle{}
	start := end.Add(-time.Duration(n) * tf).Truncate(tf)
	steps := int(tf / (5 * time.Second))
	for _, in := range Instruments {
		price := f.prices[in.Symbol]
		candles := make([]Candle, 0, n)
		for i := 0; i < n; i++ {
			c := Candle{Time: start.Add(time.Duration(i) * tf).Unix(), Open: price, High: price, Low: price}
			for s := 0; s < steps; s++ {
				price = f.walk(in, price, 5)
				c.High = math.Max(c.High, price)
				c.Low = math.Min(c.Low, price)
			}
			c.Close = price
			candles = append(candles, c)
		}
		f.prices[in.Symbol] = price
		out[in.Symbol] = candles
	}
	return out
}

// RoundToTick làm tròn giá theo bước giá, khử sai số dấu phẩy động (0.1+0.2).
func RoundToTick(price, tick float64) float64 {
	n := math.Round(price / tick)
	p := 1.0
	for i := 0; i < 10 && math.Abs(tick*p-math.Round(tick*p)) > 1e-9; i++ {
		p *= 10 // số chữ số thập phân của chính bước giá: 0.25 → 2, 0.0005 → 4
	}
	return math.Round(n*tick*p) / p
}
