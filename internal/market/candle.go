package market

import (
	"math"
	"time"
)

// Candle là nến OHLC; Time là giây unix của đầu khung, đúng định dạng Lightweight Charts cần.
type Candle struct {
	Time  int64   `json:"time"`
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

// BucketStart trả về giây unix của đầu khung chứa thời điểm tms (mili giây).
func BucketStart(tms int64, tf time.Duration) int64 {
	sec := int64(tf / time.Second)
	return (tms / 1000) / sec * sec
}

// Aggregator gộp tick thành nến theo khung tf, mỗi mã một nến đang mở.
type Aggregator struct {
	tf  time.Duration
	cur map[string]Candle
}

func NewAggregator(tf time.Duration) *Aggregator {
	return &Aggregator{tf: tf, cur: map[string]Candle{}}
}

// Seed đặt nến đang mở (khi khởi động lại từ dữ liệu trong Redis).
func (a *Aggregator) Seed(symbol string, c Candle) { a.cur[symbol] = c }

// Add cập nhật nến bằng một tick. closed khác nil khi tick này mở sang khung mới.
// Tick đến muộn (thuộc khung đã đóng) bị bỏ qua để nến cũ không bị sửa.
func (a *Aggregator) Add(t Tick) (closed *Candle, current Candle, ok bool) {
	bucket := BucketStart(t.Time, a.tf)
	c, exists := a.cur[t.Symbol]
	switch {
	case !exists:
		c = Candle{Time: bucket, Open: t.Price, High: t.Price, Low: t.Price, Close: t.Price}
	case bucket < c.Time:
		return nil, c, false
	case bucket > c.Time:
		old := c
		closed = &old
		c = Candle{Time: bucket, Open: old.Close, High: math.Max(old.Close, t.Price), Low: math.Min(old.Close, t.Price), Close: t.Price}
	default:
		c.High = math.Max(c.High, t.Price)
		c.Low = math.Min(c.Low, t.Price)
		c.Close = t.Price
	}
	a.cur[t.Symbol] = c
	return closed, c, true
}
