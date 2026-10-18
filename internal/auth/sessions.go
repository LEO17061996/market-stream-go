// Package auth giữ phiên đăng nhập và giới hạn số lần đăng nhập sai trong Redis.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNoSession = errors.New("chưa đăng nhập hoặc phiên đã hết hạn")

type Sessions struct {
	rdb          redis.UniversalClient
	TTL          time.Duration
	LoginLimit   int64
	LoginWindow  time.Duration
}

func NewSessions(rdb redis.UniversalClient) *Sessions {
	return &Sessions{rdb: rdb, TTL: 24 * time.Hour, LoginLimit: 10, LoginWindow: 10 * time.Minute}
}

// Redis chỉ lưu băm SHA-256 của token: lỡ lộ bản sao Redis thì cũng không dùng lại được phiên.
func key(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sess:" + hex.EncodeToString(sum[:])
}

func (s *Sessions) Create(ctx context.Context, userID int64) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, s.rdb.Set(ctx, key(token), userID, s.TTL).Err()
}

func (s *Sessions) Lookup(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, ErrNoSession
	}
	v, err := s.rdb.Get(ctx, key(token)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrNoSession
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

func (s *Sessions) Delete(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, key(token)).Err()
}

// AllowLogin đếm số lần thử theo IP trong một cửa sổ thời gian.
func (s *Sessions) AllowLogin(ctx context.Context, ip string) (bool, error) {
	k := "rl:login:" + ip
	pipe := s.rdb.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.ExpireNX(ctx, k, s.LoginWindow)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= s.LoginLimit, nil
}

func (s *Sessions) ResetLogin(ctx context.Context, ip string) {
	s.rdb.Del(ctx, "rl:login:"+ip)
}
