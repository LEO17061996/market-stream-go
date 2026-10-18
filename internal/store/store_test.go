package store_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/LEO17061996/market-stream-go/internal/store"
	"github.com/LEO17061996/market-stream-go/internal/testutil"
)

func seeded(t *testing.T) (*store.Store, store.User) {
	t.Helper()
	st := testutil.Store(t)
	ctx := context.Background()
	if err := st.SeedDemo(ctx, "demo", "123456", "Khách hàng Demo", 1); err != nil {
		t.Fatal(err)
	}
	u, err := st.Authenticate(ctx, "demo", "123456")
	if err != nil {
		t.Fatal(err)
	}
	return st, u
}

func TestAuthenticate(t *testing.T) {
	st, _ := seeded(t)
	ctx := context.Background()
	if _, err := st.Authenticate(ctx, "demo", "sai"); !errors.Is(err, store.ErrBadCredentials) {
		t.Fatalf("sai mật khẩu phải bị từ chối, nhận %v", err)
	}
	if _, err := st.Authenticate(ctx, "khong-co", "123456"); !errors.Is(err, store.ErrBadCredentials) {
		t.Fatalf("tên không tồn tại phải trả cùng một lỗi, nhận %v", err)
	}
}

func TestSeedIsIdempotentAndBalanced(t *testing.T) {
	st, u := seeded(t)
	ctx := context.Background()
	if err := st.SeedDemo(ctx, "demo", "123456", "Khách hàng Demo", 1); err != nil {
		t.Fatal(err)
	}
	acc, err := st.AccountByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if acc.BalanceVND <= 0 || acc.PendingVND != 20_000_000 || acc.AvailableVND != acc.BalanceVND-20_000_000 {
		t.Fatalf("số dư sau seed sai: %+v", acc)
	}
}

func TestKeysetPaginationWalksAllRowsOnce(t *testing.T) {
	st, u := seeded(t)
	ctx := context.Background()
	acc, _ := st.AccountByUser(ctx, u.ID)
	seen := map[int64]bool{}
	var before int64
	for page := 0; page < 50; page++ {
		items, err := st.Transactions(ctx, acc.ID, before, 25)
		if err != nil {
			t.Fatal(err)
		}
		for i, it := range items {
			if seen[it.ID] {
				t.Fatalf("giao dịch %d xuất hiện hai lần", it.ID)
			}
			if i > 0 && it.ID >= items[i-1].ID {
				t.Fatal("phải xếp id giảm dần")
			}
			seen[it.ID] = true
		}
		if len(items) < 25 {
			break
		}
		before = items[len(items)-1].ID
	}
	if len(seen) != 202 {
		t.Fatalf("muốn đi qua 202 giao dịch, được %d", len(seen))
	}
}

func TestWithdrawalRespectsAvailableBalance(t *testing.T) {
	st, u := seeded(t)
	ctx := context.Background()
	acc, _ := st.AccountByUser(ctx, u.ID)
	if _, err := st.RequestWithdrawal(ctx, u.ID, acc.AvailableVND+1000, ""); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("rút quá số dư khả dụng phải bị từ chối, nhận %v", err)
	}
	if _, err := st.RequestWithdrawal(ctx, u.ID, acc.AvailableVND, "rút hết"); err != nil {
		t.Fatal(err)
	}
	after, _ := st.AccountByUser(ctx, u.ID)
	if after.AvailableVND != 0 || after.BalanceVND != acc.BalanceVND {
		t.Fatalf("yêu cầu chờ duyệt phải giữ số dư, chỉ giảm khả dụng: %+v", after)
	}
}

// Mười yêu cầu rút gửi cùng lúc, số dư chỉ đủ cho đúng một số yêu cầu: không được rút quá.
func TestConcurrentWithdrawalsNeverOverdraw(t *testing.T) {
	st, u := seeded(t)
	ctx := context.Background()
	acc, _ := st.AccountByUser(ctx, u.ID)
	amount := acc.AvailableVND / 4 // đủ cho 4 yêu cầu, yêu cầu thứ 5 trở đi phải thiếu
	var ok, short atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.RequestWithdrawal(ctx, u.ID, amount, "")
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, store.ErrInsufficient):
				short.Add(1)
			default:
				t.Errorf("lỗi lạ: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 4 || short.Load() != 6 {
		t.Fatalf("muốn 4 thành công / 6 thiếu số dư, được %d / %d", ok.Load(), short.Load())
	}
	after, _ := st.AccountByUser(ctx, u.ID)
	if after.AvailableVND < 0 {
		t.Fatalf("số dư khả dụng âm: %+v", after)
	}
}
