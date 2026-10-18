// Package web nhúng giao diện Next.js đã xuất tĩnh (npm run build → web/out) vào file chạy Go.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:out
var files embed.FS

// FS trả về thư mục out; nil nếu chưa build giao diện (chỉ có .gitkeep).
func FS() fs.FS {
	sub, err := fs.Sub(files, "out")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
