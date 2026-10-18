"use client";

import { useEffect, useRef } from "react";
import {
  CandlestickSeries,
  ColorType,
  createChart,
  type IChartApi,
  type ISeriesApi,
  type Time,
  type UTCTimestamp,
} from "lightweight-charts";
import { api, applyTickToCandle, fmtPrice, type Candle, type Instrument, type Tick } from "@/lib/market";

const vnTime = (t: Time) =>
  new Date((t as number) * 1000).toLocaleTimeString("vi-VN", { timeZone: "Asia/Ho_Chi_Minh", hour: "2-digit", minute: "2-digit", hour12: false });

type Props = {
  instrument: Instrument;
  onTicks: (fn: (ticks: Tick[]) => void) => () => void;
};

/** Biểu đồ nến 1 phút: lấy lịch sử qua REST rồi tự cập nhật nến cuối theo tick WebSocket. */
export default function CandleChart({ instrument, onTicks }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const lastRef = useRef<Candle | undefined>(undefined);

  useEffect(() => {
    if (!box.current) return;
    const css = getComputedStyle(document.documentElement);
    const chart = createChart(box.current, {
      autoSize: true,
      layout: { background: { type: ColorType.Solid, color: "transparent" }, textColor: css.getPropertyValue("--muted").trim(), fontSize: 12 },
      grid: { vertLines: { color: css.getPropertyValue("--grid").trim() }, horzLines: { color: css.getPropertyValue("--grid").trim() } },
      rightPriceScale: { borderVisible: false },
      timeScale: { borderVisible: false, timeVisible: true, secondsVisible: false, tickMarkFormatter: vnTime },
      localization: { locale: "vi-VN", timeFormatter: vnTime },
      crosshair: { mode: 1 },
    });
    chartRef.current = chart;
    return () => {
      chart.remove();
      chartRef.current = null;
      seriesRef.current = null;
    };
  }, []);

  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;
    const series = chart.addSeries(CandlestickSeries, {
      upColor: "#16c784",
      downColor: "#ea3943",
      borderVisible: false,
      wickUpColor: "#16c784",
      wickDownColor: "#ea3943",
      // Thang giá viết kiểu Việt (3.953,9) cho khớp bảng giá
      priceFormat: { type: "custom", minMove: instrument.tickSize, formatter: (p: number) => fmtPrice(p, instrument.decimals) },
    });
    seriesRef.current = series;
    lastRef.current = undefined;
    let cancelled = false;
    // Tick đến trong lúc chờ lịch sử thì giữ lại, có lịch sử rồi mới áp vào để nến không bị lệch.
    const pending: Tick[] = [];

    const apply = (t: Tick) => {
      const next = applyTickToCandle(lastRef.current, t);
      if (lastRef.current && next.time < lastRef.current.time) return;
      lastRef.current = next;
      series.update({ ...next, time: next.time as UTCTimestamp });
    };

    const off = onTicks((ticks) => {
      for (const t of ticks) {
        if (t.s !== instrument.symbol) continue;
        if (lastRef.current) apply(t);
        else pending.push(t);
      }
    });

    api<Candle[]>(`/api/candles?symbol=${instrument.symbol}&limit=300`).then((candles) => {
      if (cancelled) return;
      series.setData(candles.map((c) => ({ ...c, time: c.time as UTCTimestamp })));
      lastRef.current = candles[candles.length - 1];
      pending.splice(0).forEach(apply);
      chart.timeScale().scrollToRealTime();
    });

    return () => {
      cancelled = true;
      off();
      if (chartRef.current) chart.removeSeries(series); // khi gỡ trang, biểu đồ đã bị huỷ trước
    };
  }, [instrument, onTicks]);

  return <div ref={box} className="h-full min-h-[320px] w-full" />;
}
