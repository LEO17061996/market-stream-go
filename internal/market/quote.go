package market

import (
	"math"
	"time"
)

// Quote là dòng bảng giá: giá mới nhất và thống kê trong ngày (theo giờ Việt Nam).
type Quote struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"changePct"`
	Time      int64   `json:"time"`
	Day       string  `json:"day"`
}

var vnZone = time.FixedZone("ICT", 7*3600)

// QuoteBook giữ bảng giá trong bộ nhớ của bộ sinh giá; bản sao được ghi vào Redis.
type QuoteBook struct {
	quotes map[string]Quote
}

func NewQuoteBook() *QuoteBook { return &QuoteBook{quotes: map[string]Quote{}} }

func (b *QuoteBook) Seed(q Quote) { b.quotes[q.Symbol] = q }

func (b *QuoteBook) Get(symbol string) (Quote, bool) {
	q, ok := b.quotes[symbol]
	return q, ok
}

// Apply cập nhật bảng giá bằng một tick; sang ngày mới thì mở cửa lại từ giá đầu tiên.
func (b *QuoteBook) Apply(t Tick) Quote {
	day := time.UnixMilli(t.Time).In(vnZone).Format("2006-01-02")
	q, ok := b.quotes[t.Symbol]
	if !ok || q.Day != day {
		q = Quote{Symbol: t.Symbol, Open: t.Price, High: t.Price, Low: t.Price, Day: day}
	}
	q.Price = t.Price
	q.High = math.Max(q.High, t.Price)
	q.Low = math.Min(q.Low, t.Price)
	q.Time = t.Time
	q.Change = t.Price - q.Open
	if q.Open != 0 {
		q.ChangePct = math.Round(q.Change/q.Open*10000) / 100
	}
	if in, found := Find(t.Symbol); found {
		q.Change = RoundToTick(q.Change, in.TickSize)
	}
	b.quotes[t.Symbol] = q
	return q
}

// Margin là kết quả tính ký quỹ cho n hợp đồng.
type Margin struct {
	Symbol      string  `json:"symbol"`
	Lots        float64 `json:"lots"`
	Price       float64 `json:"price"`
	NotionalUSD float64 `json:"notionalUSD"`
	MarginUSD   float64 `json:"marginUSD"`
	USDVND      float64 `json:"usdvnd"`
	MarginVND   int64   `json:"marginVND"`
}

// CalcMargin tính giá trị hợp đồng và ký quỹ ban đầu, quy đổi sang VND theo tỷ giá hiện tại.
func CalcMargin(in Instrument, lots, price, usdvnd float64) Margin {
	notional := lots * in.ContractSize * price * in.PriceToUSD
	margin := lots * in.MarginUSD
	return Margin{
		Symbol:      in.Symbol,
		Lots:        lots,
		Price:       price,
		NotionalUSD: math.Round(notional*100) / 100,
		MarginUSD:   math.Round(margin*100) / 100,
		USDVND:      usdvnd,
		MarginVND:   int64(math.Round(margin * usdvnd)),
	}
}
