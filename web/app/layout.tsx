import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Market Stream — bảng giá hàng hoá realtime (demo)",
  description: "Demo Go + Redis + WebSocket + Next.js: bảng giá và biểu đồ nến cập nhật trực tiếp.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="vi">
      <body>{children}</body>
    </html>
  );
}
