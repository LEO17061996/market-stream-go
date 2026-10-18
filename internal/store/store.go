// Package store lưu tài khoản và lịch sử nạp/rút ký quỹ.
// Chạy MySQL khi có MYSQL_DSN (CI, Docker Compose); không có thì dùng SQLite để bản demo chạy một mình được.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	ErrBadCredentials = errors.New("sai tên đăng nhập hoặc mật khẩu")
	ErrInsufficient   = errors.New("số dư khả dụng không đủ")
	ErrNotFound       = errors.New("không tìm thấy")
)

type Store struct {
	db    *sql.DB
	mysql bool
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	FullName string `json:"fullName"`
}

type Account struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	BalanceVND   int64  `json:"balanceVND"`
	PendingVND   int64  `json:"pendingWithdrawVND"`
	AvailableVND int64  `json:"availableVND"`
}

type Txn struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`   // deposit | withdraw
	AmountVND int64  `json:"amountVND"`
	Status    string `json:"status"` // pending | completed | rejected
	Note      string `json:"note"`
	CreatedAt int64  `json:"createdAt"` // unix ms
}

// Open nhận "mysql://user:pass@host:3306/db", "sqlite://đường/dẫn.db" hoặc chuỗi rỗng (SQLite file:demo.db).
func Open(ctx context.Context, dsn string) (*Store, error) {
	if strings.HasPrefix(dsn, "mysql://") {
		cfg, err := mysqlConfig(dsn)
		if err != nil {
			return nil, err
		}
		db, err := sql.Open("mysql", cfg.FormatDSN())
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(20)
		db.SetConnMaxLifetime(5 * time.Minute)
		return &Store{db: db, mysql: true}, waitPing(ctx, db)
	}
	path := strings.TrimPrefix(dsn, "sqlite://")
	if path == "" {
		path = "demo.db"
	}
	// _txlock=immediate: transaction ghi giữ khoá ghi ngay từ BEGIN, tương đương SELECT … FOR UPDATE bên MySQL.
	db, err := sql.Open("sqlite", "file:"+path+"?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, waitPing(ctx, db)
}

func mysqlConfig(dsn string) (*mysql.Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("MYSQL_DSN sai định dạng: %w", err)
	}
	cfg := mysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.Net = "tcp"
	cfg.Addr = u.Host
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	return cfg, nil
}

// waitPing chờ cơ sở dữ liệu sẵn sàng tối đa 30 giây (MySQL trong Docker khởi động chậm hơn ứng dụng).
func waitPing(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := db.PingContext(ctx)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Exec chạy một câu lệnh thô (dùng cho test dọn dữ liệu).
func (s *Store) Exec(ctx context.Context, q string) error {
	_, err := s.db.ExecContext(ctx, q)
	return err
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Dialect() string {
	if s.mysql {
		return "mysql"
	}
	return "sqlite"
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := sqliteSchema
	if s.mysql {
		stmts = mysqlSchema
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w\n%s", err, q)
		}
	}
	return nil
}

// dummyHash dùng khi tên đăng nhập không tồn tại, để thời gian phản hồi không lộ tên nào có thật.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("khong-dung"), bcrypt.DefaultCost)

func (s *Store) Authenticate(ctx context.Context, username, password string) (User, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, full_name, password_hash FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.FullName, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, ErrBadCredentials
	}
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrBadCredentials
	}
	return u, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, username, full_name FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Username, &u.FullName)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) AccountByUser(ctx context.Context, userID int64) (Account, error) {
	return accountByUser(ctx, s.db, userID, "")
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func accountByUser(ctx context.Context, q querier, userID int64, lock string) (Account, error) {
	var a Account
	err := q.QueryRowContext(ctx, `SELECT id, code, balance_vnd FROM accounts WHERE user_id = ?`+lock, userID).Scan(&a.ID, &a.Code, &a.BalanceVND)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	err = q.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_vnd), 0) FROM margin_transactions WHERE account_id = ? AND kind = 'withdraw' AND status = 'pending'`, a.ID).Scan(&a.PendingVND)
	a.AvailableVND = a.BalanceVND - a.PendingVND
	return a, err
}

// Transactions phân trang theo khoá (id < before) thay vì OFFSET: trang 1000 vẫn nhanh như trang 1,
// vì dùng thẳng chỉ mục (account_id, id).
func (s *Store) Transactions(ctx context.Context, accountID, before int64, limit int) ([]Txn, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if before <= 0 {
		before = 1<<62 - 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, amount_vnd, status, note, created_at FROM margin_transactions
		WHERE account_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, accountID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Txn{}
	for rows.Next() {
		var t Txn
		if err := rows.Scan(&t.ID, &t.Kind, &t.AmountVND, &t.Status, &t.Note, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RequestWithdrawal tạo yêu cầu rút ký quỹ chờ duyệt. Khoá dòng tài khoản để hai yêu cầu gửi cùng lúc
// không cùng thấy một số dư khả dụng rồi rút quá.
func (s *Store) RequestWithdrawal(ctx context.Context, userID, amount int64, note string) (Txn, error) {
	if amount <= 0 {
		return Txn{}, errors.New("số tiền phải lớn hơn 0")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Txn{}, err
	}
	defer tx.Rollback()
	lock := ""
	if s.mysql {
		lock = " FOR UPDATE"
	}
	acc, err := accountByUser(ctx, tx, userID, lock)
	if err != nil {
		return Txn{}, err
	}
	if amount > acc.AvailableVND {
		return Txn{}, ErrInsufficient
	}
	t := Txn{Kind: "withdraw", AmountVND: amount, Status: "pending", Note: note, CreatedAt: time.Now().UnixMilli()}
	res, err := tx.ExecContext(ctx, `INSERT INTO margin_transactions (account_id, kind, amount_vnd, status, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		acc.ID, t.Kind, t.AmountVND, t.Status, t.Note, t.CreatedAt)
	if err != nil {
		return Txn{}, err
	}
	if t.ID, err = res.LastInsertId(); err != nil {
		return Txn{}, err
	}
	return t, tx.Commit()
}

func (s *Store) Audit(ctx context.Context, userID int64, action, ip, detail string) {
	var uid any
	if userID > 0 {
		uid = userID
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit_logs (user_id, action, ip, detail, created_at) VALUES (?, ?, ?, ?, ?)`,
		uid, action, ip, detail, time.Now().UnixMilli())
}

// SeedDemo tạo tài khoản demo và 200 giao dịch trong 180 ngày. Chạy trong một transaction:
// nhiều bản máy chủ khởi động cùng lúc thì bản sau gặp trùng khoá và bỏ qua.
func (s *Store) SeedDemo(ctx context.Context, username, password, fullName string, seed uint64) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&n); err != nil || n > 0 {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (username, password_hash, full_name, created_at) VALUES (?, ?, ?, ?)`, username, string(hash), fullName, now.UnixMilli())
	if err != nil {
		if isDuplicate(err) {
			return nil
		}
		return err
	}
	userID, _ := res.LastInsertId()

	rng := rand.New(rand.NewPCG(seed, seed+1))
	type row struct {
		kind   string
		amount int64
		status string
		note   string
		at     int64
	}
	rows := make([]row, 0, 202)
	var balance int64
	start := now.Add(-180 * 24 * time.Hour)
	for i := 0; i < 200; i++ {
		at := start.Add(time.Duration(i) * 21 * time.Hour).Add(time.Duration(rng.IntN(3600)) * time.Second)
		amount := int64(5+rng.IntN(76)) * 1_000_000
		if balance > 200_000_000 && rng.Float64() < 0.45 {
			rows = append(rows, row{"withdraw", amount, "completed", "Rút về tài khoản ngân hàng", at.UnixMilli()})
			balance -= amount
		} else {
			rows = append(rows, row{"deposit", amount, "completed", "Nạp ký quỹ qua chuyển khoản", at.UnixMilli()})
			balance += amount
		}
	}
	rows = append(rows,
		row{"deposit", 15_000_000, "rejected", "Sai nội dung chuyển khoản, đã hoàn tiền", now.Add(-3 * time.Hour).UnixMilli()},
		row{"withdraw", 20_000_000, "pending", "Rút về tài khoản ngân hàng", now.Add(-time.Hour).UnixMilli()},
	)
	res, err = tx.ExecContext(ctx, `INSERT INTO accounts (user_id, code, balance_vnd, updated_at) VALUES (?, ?, ?, ?)`,
		userID, fmt.Sprintf("TK%06d", userID), balance, now.UnixMilli())
	if err != nil {
		return err
	}
	accountID, _ := res.LastInsertId()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO margin_transactions (account_id, kind, amount_vnd, status, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, accountID, r.kind, r.amount, r.status, r.note, r.at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return true
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
