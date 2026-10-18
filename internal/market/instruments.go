// Package market chứa danh mục hợp đồng, bộ sinh giá giả lập và gộp nến.
// Mọi giá ở đây là GIẢ LẬP (random walk), thông số hợp đồng chỉ để minh hoạ.
package market

// Instrument là một hợp đồng tương lai (hoặc tỷ giá dùng để quy đổi).
type Instrument struct {
	Symbol       string  `json:"symbol"`
	Name         string  `json:"name"`
	Exchange     string  `json:"exchange"`
	Unit         string  `json:"unit"`
	Decimals     int     `json:"decimals"`
	TickSize     float64 `json:"tickSize"`
	ContractSize float64 `json:"contractSize"` // số đơn vị hàng hoá / 1 hợp đồng; 0 = không giao dịch (tỷ giá)
	PriceToUSD   float64 `json:"priceToUSD"`   // 1 nếu giá tính bằng USD, 0.01 nếu tính bằng cent
	MarginUSD    float64 `json:"marginUSD"`    // ký quỹ ban đầu / 1 hợp đồng (minh hoạ)

	startPrice float64
	sigma      float64 // biến động mỗi giây
}

// USDVND là mã tỷ giá dùng để quy đổi ký quỹ sang VND.
const USDVND = "USDVND"

// Instruments là danh mục cố định của bản demo.
var Instruments = []Instrument{
	{Symbol: "GC", Name: "Vàng", Exchange: "COMEX", Unit: "USD/oz", Decimals: 1, TickSize: 0.1, ContractSize: 100, PriceToUSD: 1, MarginUSD: 16500, startPrice: 4010, sigma: 0.00012},
	{Symbol: "SI", Name: "Bạc", Exchange: "COMEX", Unit: "USD/oz", Decimals: 3, TickSize: 0.005, ContractSize: 5000, PriceToUSD: 1, MarginUSD: 22000, startPrice: 48.2, sigma: 0.0002},
	{Symbol: "HG", Name: "Đồng", Exchange: "COMEX", Unit: "USD/lb", Decimals: 4, TickSize: 0.0005, ContractSize: 25000, PriceToUSD: 1, MarginUSD: 9500, startPrice: 5.02, sigma: 0.00015},
	{Symbol: "CL", Name: "Dầu thô WTI", Exchange: "NYMEX", Unit: "USD/thùng", Decimals: 2, TickSize: 0.01, ContractSize: 1000, PriceToUSD: 1, MarginUSD: 6200, startPrice: 62.4, sigma: 0.0002},
	{Symbol: "BRN", Name: "Dầu Brent", Exchange: "ICE EU", Unit: "USD/thùng", Decimals: 2, TickSize: 0.01, ContractSize: 1000, PriceToUSD: 1, MarginUSD: 6000, startPrice: 66.1, sigma: 0.0002},
	{Symbol: "ZC", Name: "Ngô", Exchange: "CBOT", Unit: "cent/giạ", Decimals: 2, TickSize: 0.25, ContractSize: 5000, PriceToUSD: 0.01, MarginUSD: 1100, startPrice: 421.5, sigma: 0.00012},
	{Symbol: "ZS", Name: "Đậu tương", Exchange: "CBOT", Unit: "cent/giạ", Decimals: 2, TickSize: 0.25, ContractSize: 5000, PriceToUSD: 0.01, MarginUSD: 2100, startPrice: 1046.25, sigma: 0.0001},
	{Symbol: "KC", Name: "Cà phê Arabica", Exchange: "ICE US", Unit: "cent/lb", Decimals: 2, TickSize: 0.05, ContractSize: 37500, PriceToUSD: 0.01, MarginUSD: 13000, startPrice: 382.4, sigma: 0.00022},
	{Symbol: USDVND, Name: "Tỷ giá USD/VND", Exchange: "Tham chiếu", Unit: "VND", Decimals: 0, TickSize: 1, startPrice: 26310, sigma: 0.00002},
}

// Find tìm hợp đồng theo mã.
func Find(symbol string) (Instrument, bool) {
	for _, in := range Instruments {
		if in.Symbol == symbol {
			return in, true
		}
	}
	return Instrument{}, false
}
