// Package testutil dựng Redis và cơ sở dữ liệu cho test.
// Redis: dùng REDIS_TEST_URL nếu có (CI), không thì miniredis trong bộ nhớ.
// CSDL: dùng MYSQL_TEST_DSN nếu có (CI), không thì SQLite file tạm.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/LEO17061996/market-stream-go/internal/store"
)

func Redis(t *testing.T) *redis.Client {
	t.Helper()
	if url := os.Getenv("REDIS_TEST_URL"); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		rdb := redis.NewClient(opt)
		if err := rdb.FlushDB(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rdb.Close() })
		return rdb
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func Store(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		dsn = "sqlite://" + filepath.Join(t.TempDir(), "test.db")
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if st.Dialect() == "mysql" {
		ResetMySQL(t, st)
	}
	return st
}

// ResetMySQL xoá dữ liệu giữa các test dùng chung một MySQL. Xoá bảng con trước bảng cha để không vướng khoá ngoại
// (SET FOREIGN_KEY_CHECKS chỉ có tác dụng trên một kết nối, mà database/sql dùng nhiều kết nối).
func ResetMySQL(t *testing.T, st *store.Store) {
	t.Helper()
	for _, q := range []string{"DELETE FROM audit_logs", "DELETE FROM margin_transactions", "DELETE FROM accounts", "DELETE FROM users"} {
		if err := st.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
}
