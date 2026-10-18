# Market Stream — bảng giá hàng hoá realtime (Go + Redis + WebSocket + Next.js)

[![CI](https://github.com/LEO17061996/market-stream-go/actions/workflows/ci.yml/badge.svg)](https://github.com/LEO17061996/market-stream-go/actions/workflows/ci.yml)

Demo kỹ thuật một hệ thống đẩy giá trực tiếp cho cổng khách hàng giao dịch hàng hoá: bộ sinh giá ghi vào Redis, nhiều máy chủ Go (Gin) nhận qua Redis pub/sub rồi đẩy về trình duyệt bằng WebSocket; giao diện Next.js vẽ biểu đồ nến bằng Lightweight Charts (thư viện mã nguồn mở của TradingView); tài khoản và lịch sử nạp/rút ký quỹ lưu trong MySQL.

> Giá là **giả lập** (random walk), không phải dữ liệu thị trường. Thông số hợp đồng và ký quỹ chỉ để minh hoạ.

![Bảng giá và biểu đồ nến](docs/bang-gia.png)

## Chạy

```bash
docker compose up -d --build
```

Mở http://localhost:8088 — tài khoản ký quỹ demo `demo` / `123456`.

Compose dựng đủ: MySQL 8.4, Redis 7, 1 bộ sinh giá, **2 bản máy chủ** và Nginx đứng trước. Header `X-Upstream` cho biết bản nào đã trả lời; `GET /api/health` trả tên máy, số client WebSocket, độ trễ từ lúc sinh tick tới lúc máy chủ nhận.

## Kiến trúc

```
cmd/feed ──► Redis ──pub/sub──► cmd/server (×N) ──WebSocket──► trình duyệt
   │          ├ md:quotes        (Gin, REST API)                (Next.js, Lightweight Charts)
   │          ├ md:candles:1m:*        │
   │          └ sess:*, rl:login:*     └──► MySQL (tài khoản, nạp/rút ký quỹ, nhật ký)
   └ sinh tick 250 ms, gộp nến 1 phút
```

| Phần | Làm gì | File |
|---|---|---|
| Bộ sinh giá | Chuyển động Brown hình học, làm tròn theo bước giá từng hợp đồng; khởi động lại thì nối tiếp giá cũ trong Redis, Redis trống thì sinh 300 nến lịch sử | `internal/market`, `internal/feeder` |
| Redis | Mỗi bước ghi bảng giá (hash), nến (list giới hạn 600), phát tick (pub/sub) trong một `MULTI/EXEC`; cache bảng giá cho REST; phiên đăng nhập; giới hạn đăng nhập sai | `internal/bus`, `internal/auth` |
| WebSocket | Mỗi máy chủ nghe Redis một lần rồi phát cho mọi client; client theo dõi tất cả dùng chung một bản JSON mã hoá sẵn, client chỉ theo dõi vài mã thì lọc riêng; **client đọc chậm bị ngắt** để không làm chậm người khác; ping 25 s, đo độ trễ khứ hồi; chỉ nhận kết nối cùng nguồn gốc | `internal/stream` |
| REST API | Danh mục, bảng giá, nến, tính ký quỹ theo giá và tỷ giá mới nhất, đăng nhập, tài khoản, lịch sử, yêu cầu rút | `internal/api` |
| MySQL | Khoá dòng tài khoản (`SELECT … FOR UPDATE`) khi rút ký quỹ; phân trang theo khoá (`id < mốc`) trên chỉ mục `(account_id, id)` thay vì OFFSET; ràng buộc CHECK cho số dư và trạng thái | `internal/store` |
| Giao diện | Bảng giá nhấp nháy theo chiều giá, biểu đồ nến cập nhật theo tick, tự nối lại khi mất kết nối (chờ tăng dần, có ngẫu nhiên), máy tính ký quỹ, lịch sử "Xem thêm" | `web/` |
| Triển khai | Docker nhiều tầng (Next.js → Go → distroless, user không phải root), Nginx proxy WebSocket và tự hỏi lại DNS khi bản máy chủ đổi IP, GitHub Actions | `Dockerfile`, `deploy/nginx.conf`, `.github/workflows/ci.yml` |

| Tài khoản ký quỹ | Điện thoại |
|---|---|
| ![Tài khoản](docs/tai-khoan.png) | ![Điện thoại](docs/dien-thoai.png) |

## Kiểm thử

```bash
go test ./...                       # Redis trong bộ nhớ (miniredis) + SQLite
MYSQL_TEST_DSN=mysql://root:root@127.0.0.1:3306/market_test \
REDIS_TEST_URL=redis://127.0.0.1:6379/1 go test -race -p 1 ./...   # MySQL + Redis thật, như CI
```

Các ca chính:

- Giá luôn nằm trên bước giá (kể cả bước 0,25 và 0,0005), cùng seed thì cùng chuỗi giá; nến mới mở bằng giá đóng nến trước, tick đến muộn không sửa nến đã đóng; bảng giá mở cửa lại lúc 0 giờ Việt Nam.
- Đầu cuối: ghi tick vào Redis → hub nghe pub/sub → client WebSocket nhận đúng mã đã theo dõi; ping/pong; trang khác nguồn gốc bị từ chối.
- Client đọc chậm bị ngắt mà client khác vẫn nhận đủ; gửi tin cho client vừa bị gỡ không panic.
- 10 yêu cầu rút gửi cùng lúc, số dư đủ cho 4: đúng 4 thành công, 6 bị từ chối, không âm.
- Phân trang theo khoá đi qua 202 giao dịch, mỗi dòng đúng một lần.
- Đăng nhập: sai mật khẩu và sai tên trả cùng lỗi; quá 10 lần trong 10 phút bị chặn kể cả đúng mật khẩu; đăng xuất thì phiên mất hiệu lực; POST không phải JSON bị chặn.

CI chạy `go vet`, test có `-race` trên MySQL 8.4 + Redis 7 thật, kiểm kiểu và build giao diện, build ảnh Docker.

## Bản chạy một tiến trình

Không có MySQL thì máy chủ dùng SQLite; đặt `FEED_EMBEDDED=1` để bộ sinh giá chạy chung tiến trình (dùng cho gói miễn phí của Render, xem `render.yaml`). Biến môi trường: `PORT`, `REDIS_URL`, `MYSQL_DSN` (`mysql://user:pass@host:3306/db`), `FEED_EMBEDDED`, `FEED_INTERVAL_MS`.

## Giới hạn của bản demo

- Không nối nguồn giá thật (CME, ICE, LME qua nhà cung cấp dữ liệu), không đặt lệnh, không có vị thế và lãi/lỗ.
- Yêu cầu rút chỉ tạo ở trạng thái chờ duyệt, chưa có màn duyệt cho nhân viên.
- Chưa có module CRM và phân cấp môi giới (IB).

## Giấy phép

MIT
