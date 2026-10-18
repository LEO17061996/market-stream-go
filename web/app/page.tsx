"use client";

import dynamic from "next/dynamic";
import { useState } from "react";
import PriceBoard from "@/components/PriceBoard";
import { Disclaimer, Header } from "@/components/Shell";
import { fmtPrice } from "@/lib/market";
import { useMarket } from "@/lib/useMarket";

// Lightweight Charts cần DOM nên chỉ nạp ở trình duyệt.
const CandleChart = dynamic(() => import("@/components/CandleChart"), { ssr: false });

export default function Home() {
  const { instruments, quotes, status, rttMs, ticksPerSec, onTicks } = useMarket();
  const [selected, setSelected] = useState("GC");
  const ins = instruments.find((i) => i.symbol === selected);
  const q = quotes[selected];

  return (
    <>
      <Header status={status} rttMs={rttMs} ticksPerSec={ticksPerSec} />
      <Disclaimer />
      {/* Bảng giá cần ~640px mới không phải cuộn ngang: màn hẹp hơn thì bảng và biểu đồ xếp chồng. */}
      <main className="grid gap-4 p-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,640px),1fr))]">
        <section className="card">
          <h1 className="mb-2 px-1 text-base font-semibold">Bảng giá hàng hoá</h1>
          <PriceBoard instruments={instruments} quotes={quotes} selected={selected} onSelect={setSelected} />
          <p className="mt-2 px-1 text-xs text-[var(--muted)]">Bấm vào một dòng để xem biểu đồ. Thay đổi tính từ giá mở cửa trong ngày (giờ Việt Nam).</p>
        </section>
        <section className="card flex min-h-[420px] flex-col">
          {ins && (
            <div className="mb-2 flex flex-wrap items-baseline gap-x-3 px-1">
              <h2 className="text-base font-semibold">
                {ins.name} · {ins.symbol}
              </h2>
              <span className="text-xs text-[var(--muted)]">
                {ins.exchange} · nến 1 phút · {ins.unit}
              </span>
              {q && (
                <span className={`ml-auto text-lg font-semibold tabular-nums ${q.change >= 0 ? "text-[var(--up)]" : "text-[var(--down)]"}`}>
                  {fmtPrice(q.price, ins.decimals)}
                </span>
              )}
            </div>
          )}
          <div className="min-h-0 flex-1">{ins && <CandleChart instrument={ins} onTicks={onTicks} />}</div>
        </section>
      </main>
    </>
  );
}
