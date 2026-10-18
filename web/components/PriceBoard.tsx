"use client";

import { useEffect, useRef, useState } from "react";
import { fmtPrice, fmtTime, type Instrument, type Quote } from "@/lib/market";

type Props = {
  instruments: Instrument[];
  quotes: Record<string, Quote>;
  selected: string;
  onSelect: (symbol: string) => void;
};

export default function PriceBoard({ instruments, quotes, selected, onSelect }: Props) {
  return (
    <div className="@container overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="text-left text-[var(--muted)]">
            <th className="px-3 py-2 font-medium">Mã</th>
            <th className="hidden px-3 py-2 font-medium @lg:table-cell">Hàng hoá</th>
            <th className="px-3 py-2 text-right font-medium">Giá</th>
            <th className="hidden px-3 py-2 text-right font-medium @lg:table-cell">+/−</th>
            <th className="px-3 py-2 text-right font-medium">%</th>
            <th className="hidden px-3 py-2 text-right font-medium @2xl:table-cell">Cao</th>
            <th className="hidden px-3 py-2 text-right font-medium @2xl:table-cell">Thấp</th>
            <th className="hidden px-3 py-2 text-right font-medium @2xl:table-cell">Cập nhật</th>
          </tr>
        </thead>
        <tbody>
          {instruments.map((ins) => (
            <Row key={ins.symbol} ins={ins} q={quotes[ins.symbol]} active={ins.symbol === selected} onSelect={onSelect} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Row({ ins, q, active, onSelect }: { ins: Instrument; q?: Quote; active: boolean; onSelect: (s: string) => void }) {
  const prev = useRef<number | undefined>(undefined);
  const [flash, setFlash] = useState<{ dir: "up" | "down"; n: number } | null>(null);

  useEffect(() => {
    if (q === undefined) return;
    if (prev.current !== undefined && q.price !== prev.current) {
      setFlash((f) => ({ dir: q.price > prev.current! ? "up" : "down", n: (f?.n ?? 0) + 1 }));
    }
    prev.current = q.price;
  }, [q]);

  const tradable = ins.contractSize > 0;
  const tone = !q || q.change === 0 ? "" : q.change > 0 ? "text-[var(--up)]" : "text-[var(--down)]";
  return (
    <tr
      onClick={() => tradable && onSelect(ins.symbol)}
      className={`border-t border-[var(--line)] ${tradable ? "cursor-pointer hover:bg-[var(--hover)]" : ""} ${active ? "bg-[var(--hover)]" : ""}`}
    >
      <td className="px-3 py-2">
        <div className="font-semibold">{ins.symbol}</div>
        {/* Khung hẹp: tên hàng hoá nằm dưới mã thay cho cột riêng */}
        <div className="text-xs text-[var(--muted)] @lg:hidden">{ins.name}</div>
        <div className="hidden whitespace-nowrap text-xs text-[var(--muted)] @lg:block">{ins.exchange}</div>
      </td>
      <td className="hidden px-3 py-2 @lg:table-cell">
        <div className="whitespace-nowrap">{ins.name}</div>
        <div className="whitespace-nowrap text-xs text-[var(--muted)]">{ins.unit}</div>
      </td>
      <td className="px-3 py-2 text-right tabular-nums">
        <span key={flash?.n} className={`rounded px-1 font-semibold ${flash ? (flash.dir === "up" ? "flash-up" : "flash-down") : ""}`}>
          {q ? fmtPrice(q.price, ins.decimals) : "–"}
        </span>
      </td>
      <td className={`hidden px-3 py-2 text-right tabular-nums @lg:table-cell ${tone}`}>{q ? (q.change > 0 ? "+" : "") + fmtPrice(q.change, ins.decimals) : "–"}</td>
      <td className={`px-3 py-2 text-right tabular-nums ${tone}`}>{q ? (q.changePct > 0 ? "+" : "") + q.changePct.toLocaleString("vi-VN", { minimumFractionDigits: 2 }) + "%" : "–"}</td>
      <td className="hidden px-3 py-2 text-right tabular-nums text-[var(--muted)] @2xl:table-cell">{q ? fmtPrice(q.high, ins.decimals) : "–"}</td>
      <td className="hidden px-3 py-2 text-right tabular-nums text-[var(--muted)] @2xl:table-cell">{q ? fmtPrice(q.low, ins.decimals) : "–"}</td>
      <td className="hidden px-3 py-2 text-right tabular-nums text-[var(--muted)] @2xl:table-cell">{q ? fmtTime(q.time) : "–"}</td>
    </tr>
  );
}
