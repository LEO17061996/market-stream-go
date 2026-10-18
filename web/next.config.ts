import type { NextConfig } from "next";

// Xuất tĩnh ra web/out; máy chủ Go nhúng thư mục này và phục vụ cùng cổng với /api và /ws.
const config: NextConfig = {
  output: "export",
  trailingSlash: true,
  images: { unoptimized: true },
};

export default config;
