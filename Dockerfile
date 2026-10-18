# 1) Build giao diện Next.js thành file tĩnh
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 2) Build hai file chạy Go, nhúng giao diện vào server
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/out ./web/out
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/feed ./cmd/feed

# 3) Ảnh chạy tối giản, không có shell, chạy bằng user không phải root
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ /app/
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/server"]
