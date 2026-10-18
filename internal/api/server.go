// Package api là REST API (Gin) và điểm nối WebSocket, đồng thời phục vụ giao diện Next.js đã build sẵn.
package api

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LEO17061996/market-stream-go/internal/auth"
	"github.com/LEO17061996/market-stream-go/internal/bus"
	"github.com/LEO17061996/market-stream-go/internal/market"
	"github.com/LEO17061996/market-stream-go/internal/store"
	"github.com/LEO17061996/market-stream-go/internal/stream"
)

const cookieName = "sid"

type Server struct {
	Bus      *bus.Bus
	Hub      *stream.Hub
	Store    *store.Store
	Sessions *auth.Sessions
	Static   fs.FS // giao diện đã build; nil khi chạy test
	Log      *slog.Logger
}

func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), securityHeaders)

	r.GET("/ws", func(c *gin.Context) { s.Hub.ServeWS(c.Writer, c.Request) })

	api := r.Group("/api", requireJSONForWrites)
	api.GET("/health", s.health)
	api.GET("/instruments", s.instruments)
	api.GET("/quotes", s.quotes)
	api.GET("/candles", s.candles)
	api.POST("/margin", s.optionalUser, s.margin)
	api.POST("/login", s.login)
	api.POST("/logout", s.logout)

	me := api.Group("", s.requireUser)
	me.GET("/me", s.me)
	me.GET("/transactions", s.transactions)
	me.POST("/withdrawals", s.withdraw)

	r.NoRoute(s.static)
	return r
}

func securityHeaders(c *gin.Context) {
	h := c.Writer.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "same-origin")
	c.Next()
}

// requireJSONForWrites: trang web lạ không gửi được JSON sang đây mà không qua preflight CORS,
// nên cookie SameSite=Lax + bắt buộc Content-Type JSON là đủ chặn CSRF cho bản demo.
func requireJSONForWrites(c *gin.Context) {
	if c.Request.Method == http.MethodPost && !strings.HasPrefix(c.ContentType(), "application/json") {
		c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"error": "Chỉ nhận Content-Type: application/json"})
		return
	}
	c.Next()
}

func fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c, 2*time.Second)
	defer cancel()
	redisOK := s.Bus.Client().Ping(ctx).Err() == nil
	dbOK := s.Store.Ping(ctx) == nil
	host, _ := os.Hostname()
	code := http.StatusOK
	if !redisOK || !dbOK {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, gin.H{"ok": redisOK && dbOK, "redis": redisOK, "db": dbOK, "dbDialect": s.Store.Dialect(), "instance": host, "stream": s.Hub.Stats()})
}

func (s *Server) instruments(c *gin.Context) {
	c.JSON(http.StatusOK, market.Instruments)
}

func (s *Server) quotes(c *gin.Context) {
	qs, err := s.Bus.Quotes(c)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "Không đọc được bảng giá")
		return
	}
	out := make([]market.Quote, 0, len(qs))
	for _, in := range market.Instruments {
		if q, ok := qs[in.Symbol]; ok {
			out = append(out, q)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) candles(c *gin.Context) {
	sym := c.Query("symbol")
	if _, ok := market.Find(sym); !ok {
		fail(c, http.StatusBadRequest, "Mã hợp đồng không hợp lệ")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "300"))
	cs, err := s.Bus.Candles(c, sym, limit)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "Không đọc được dữ liệu nến")
		return
	}
	c.JSON(http.StatusOK, cs)
}

type marginReq struct {
	Symbol string  `json:"symbol"`
	Lots   float64 `json:"lots"`
}

// margin tính ký quỹ theo giá và tỷ giá mới nhất trong cache; đã đăng nhập thì kèm số hợp đồng tối đa mở được.
func (s *Server) margin(c *gin.Context) {
	var req marginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "Dữ liệu không hợp lệ")
		return
	}
	in, ok := market.Find(req.Symbol)
	if !ok || in.ContractSize == 0 {
		fail(c, http.StatusBadRequest, "Mã hợp đồng không hợp lệ")
		return
	}
	if req.Lots <= 0 || req.Lots > 1000 || math.Round(req.Lots) != req.Lots {
		fail(c, http.StatusBadRequest, "Số hợp đồng phải là số nguyên từ 1 đến 1000")
		return
	}
	qs, err := s.Bus.Quotes(c)
	if err != nil || qs[in.Symbol].Price == 0 || qs[market.USDVND].Price == 0 {
		fail(c, http.StatusServiceUnavailable, "Chưa có giá để tính")
		return
	}
	m := market.CalcMargin(in, req.Lots, qs[in.Symbol].Price, qs[market.USDVND].Price)
	resp := gin.H{"margin": m}
	if uid, ok := c.Get("userID"); ok {
		if acc, err := s.Store.AccountByUser(c, uid.(int64)); err == nil {
			perLot := market.CalcMargin(in, 1, m.Price, m.USDVND).MarginVND
			resp["availableVND"] = acc.AvailableVND
			resp["maxLots"] = acc.AvailableVND / perLot
		}
	}
	c.JSON(http.StatusOK, resp)
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		fail(c, http.StatusBadRequest, "Nhập tên đăng nhập và mật khẩu")
		return
	}
	ip := c.ClientIP()
	allowed, err := s.Sessions.AllowLogin(c, ip)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "Hệ thống tạm thời không đăng nhập được")
		return
	}
	if !allowed {
		fail(c, http.StatusTooManyRequests, "Thử sai quá nhiều lần, vui lòng đợi 10 phút")
		return
	}
	u, err := s.Store.Authenticate(c, req.Username, req.Password)
	if errors.Is(err, store.ErrBadCredentials) {
		s.Store.Audit(c, 0, "login_failed", ip, req.Username)
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "Lỗi hệ thống")
		return
	}
	token, err := s.Sessions.Create(c, u.ID)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "Không tạo được phiên đăng nhập")
		return
	}
	s.Sessions.ResetLogin(c, ip)
	s.Store.Audit(c, u.ID, "login", ip, "")
	s.setCookie(c, token, int(s.Sessions.TTL.Seconds()))
	c.JSON(http.StatusOK, u)
}

func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie(cookieName); err == nil {
		_ = s.Sessions.Delete(c, token)
	}
	s.setCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (s *Server) setCookie(c *gin.Context, value string, maxAge int) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, value, maxAge, "/", "", secure, true)
}

func (s *Server) userFromCookie(c *gin.Context) (int64, error) {
	token, err := c.Cookie(cookieName)
	if err != nil {
		return 0, auth.ErrNoSession
	}
	return s.Sessions.Lookup(c, token)
}

func (s *Server) optionalUser(c *gin.Context) {
	if uid, err := s.userFromCookie(c); err == nil {
		c.Set("userID", uid)
	}
	c.Next()
}

func (s *Server) requireUser(c *gin.Context) {
	uid, err := s.userFromCookie(c)
	if err != nil {
		fail(c, http.StatusUnauthorized, auth.ErrNoSession.Error())
		return
	}
	c.Set("userID", uid)
	c.Next()
}

func (s *Server) me(c *gin.Context) {
	uid := c.GetInt64("userID")
	u, err := s.Store.UserByID(c, uid)
	if err != nil {
		fail(c, http.StatusUnauthorized, auth.ErrNoSession.Error())
		return
	}
	acc, err := s.Store.AccountByUser(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "Không đọc được tài khoản")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": u, "account": acc})
}

func (s *Server) transactions(c *gin.Context) {
	acc, err := s.Store.AccountByUser(c, c.GetInt64("userID"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "Không đọc được tài khoản")
		return
	}
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := s.Store.Transactions(c, acc.ID, before, limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, "Không đọc được lịch sử")
		return
	}
	var next int64
	if len(items) > 0 && len(items) == limit {
		next = items[len(items)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "nextBefore": next})
}

type withdrawReq struct {
	AmountVND int64  `json:"amountVND"`
	Note      string `json:"note"`
}

func (s *Server) withdraw(c *gin.Context) {
	var req withdrawReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AmountVND < 100_000 || req.AmountVND%1000 != 0 {
		fail(c, http.StatusBadRequest, "Số tiền rút tối thiểu 100.000 đ, làm tròn nghìn đồng")
		return
	}
	if len([]rune(req.Note)) > 200 {
		fail(c, http.StatusBadRequest, "Ghi chú tối đa 200 ký tự")
		return
	}
	uid := c.GetInt64("userID")
	note := strings.TrimSpace(req.Note)
	if note == "" {
		note = "Rút về tài khoản ngân hàng"
	}
	t, err := s.Store.RequestWithdrawal(c, uid, req.AmountVND, note)
	if errors.Is(err, store.ErrInsufficient) {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "Không tạo được yêu cầu rút")
		return
	}
	s.Store.Audit(c, uid, "withdraw_request", c.ClientIP(), strconv.FormatInt(req.AmountVND, 10))
	c.JSON(http.StatusCreated, t)
}

// static phục vụ giao diện Next.js xuất tĩnh. File băm trong _next/static được cache lâu; HTML thì không.
func (s *Server) static(c *gin.Context) {
	p := c.Request.URL.Path
	if strings.HasPrefix(p, "/api/") || s.Static == nil {
		fail(c, http.StatusNotFound, "Không tìm thấy")
		return
	}
	if strings.HasPrefix(p, "/_next/static/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
	name := strings.TrimPrefix(p, "/")
	check := name
	if name == "" || strings.HasSuffix(name, "/") {
		check += "index.html" // thư mục: để FileServer tự trả index.html, gọi thẳng /index.html sẽ bị nó chuyển hướng vòng
	}
	if _, err := fs.Stat(s.Static, check); err != nil {
		if _, err2 := fs.Stat(s.Static, name+"/index.html"); err2 == nil {
			c.Redirect(http.StatusMovedPermanently, p+"/")
			return
		}
		page, _ := fs.ReadFile(s.Static, "404.html")
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", page)
		return
	}
	c.FileFromFS(p, http.FS(s.Static))
}
