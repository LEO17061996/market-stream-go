"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { Status } from "@/lib/useMarket";

const STATUS: Record<Status, { label: string; dot: string }> = {
  connecting: { label: "Đang kết nối", dot: "bg-yellow-400" },
  live: { label: "Trực tiếp", dot: "bg-[var(--up)]" },
  reconnecting: { label: "Mất kết nối, đang nối lại", dot: "bg-[var(--down)]" },
};

export function Header({ status, rttMs, ticksPerSec }: { status?: Status; rttMs?: number | null; ticksPerSec?: number }) {
  const path = usePathname();
  const link = (href: string, label: string) => (
    <Link href={href} className={`rounded-md px-3 py-1.5 ${path === href ? "bg-[var(--hover)] text-[var(--ink)]" : "text-[var(--muted)] hover:text-[var(--ink)]"}`}>
      {label}
    </Link>
  );
  return (
    <header className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-[var(--line)] px-4 py-3">
      <span className="font-semibold">Market Stream</span>
      <nav className="flex flex-wrap gap-1 text-sm">
        {link("/", "Bảng giá")}
        {link("/tai-khoan/", "Tài khoản ký quỹ")}
      </nav>
      {status && (
        <div className="ml-auto flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--muted)]">
          <span className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${STATUS[status].dot}`} />
            {STATUS[status].label}
          </span>
          {rttMs != null && <span title="Thời gian khứ hồi ping/pong qua WebSocket">Độ trễ {rttMs} ms</span>}
          {ticksPerSec != null && <span>{ticksPerSec} tick/giây</span>}
        </div>
      )}
    </header>
  );
}

export function Disclaimer() {
  return (
    <p className="mx-4 mt-3 rounded-md border border-[var(--line)] px-3 py-2 text-xs text-[var(--muted)]">
      Bản demo kỹ thuật: giá là <b>giả lập</b> (random walk), không phải dữ liệu thị trường; thông số hợp đồng và ký quỹ chỉ để minh hoạ.
    </p>
  );
}
