"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api, applyTick, type Instrument, type Quote, type Tick } from "./market";

export type Status = "connecting" | "live" | "reconnecting";

type Listener = (ticks: Tick[]) => void;

/**
 * Nối WebSocket /ws, giữ bảng giá cập nhật theo tick.
 * Mất kết nối thì nối lại với thời gian chờ tăng dần (0,5 s → 30 s, có ngẫu nhiên để hàng nghìn
 * trình duyệt không cùng ập vào một lúc), nối lại xong lấy lại ảnh chụp bảng giá để lấp khoảng trống.
 */
export function useMarket() {
  const [instruments, setInstruments] = useState<Instrument[]>([]);
  const [quotes, setQuotes] = useState<Record<string, Quote>>({});
  const [status, setStatus] = useState<Status>("connecting");
  const [rttMs, setRttMs] = useState<number | null>(null);
  const [ticksPerSec, setTicksPerSec] = useState(0);
  const listeners = useRef(new Set<Listener>());

  useEffect(() => {
    let ws: WebSocket | null = null;
    let stopped = false;
    let attempt = 0;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let pingTimer: ReturnType<typeof setInterval> | undefined;
    let pingId = 0;
    const sentAt = new Map<number, number>();
    let tickCount = 0;
    const rateTimer = setInterval(() => {
      setTicksPerSec(tickCount);
      tickCount = 0;
    }, 1000);

    const snapshot = () =>
      api<Quote[]>("/api/quotes")
        .then((list) => setQuotes(Object.fromEntries(list.map((q) => [q.symbol, q]))))
        .catch(() => {});

    api<Instrument[]>("/api/instruments").then(setInstruments).catch(() => {});

    const connect = () => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(`${proto}://${location.host}/ws`);
      ws.onopen = () => {
        attempt = 0;
        setStatus("live");
        ws?.send(JSON.stringify({ op: "subscribe", symbols: ["*"] }));
        snapshot();
        pingTimer = setInterval(() => {
          pingId += 1;
          sentAt.set(pingId, performance.now());
          ws?.send(JSON.stringify({ op: "ping", id: pingId }));
        }, 5000);
      };
      ws.onmessage = (ev) => {
        const msg = JSON.parse(ev.data);
        if (msg.type === "ticks") {
          const ticks: Tick[] = msg.data;
          tickCount += ticks.length;
          setQuotes((prev) => {
            const next = { ...prev };
            for (const t of ticks) next[t.s] = applyTick(prev[t.s], t);
            return next;
          });
          listeners.current.forEach((fn) => fn(ticks));
        } else if (msg.type === "pong" && sentAt.has(msg.id)) {
          setRttMs(Math.round(performance.now() - sentAt.get(msg.id)!));
          sentAt.delete(msg.id);
        }
      };
      ws.onclose = () => {
        clearInterval(pingTimer);
        if (stopped) return;
        setStatus("reconnecting");
        const delay = Math.min(30_000, 500 * 2 ** attempt) * (0.7 + Math.random() * 0.6);
        attempt += 1;
        retryTimer = setTimeout(connect, delay);
      };
    };

    snapshot();
    connect();
    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      clearInterval(pingTimer);
      clearInterval(rateTimer);
      ws?.close();
    };
  }, []);

  // Giữ nguyên tham chiếu để component dùng trong useEffect không phải đăng ký lại sau mỗi tick.
  const onTicks = useCallback((fn: Listener) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  }, []);

  return { instruments, quotes, status, rttMs, ticksPerSec, onTicks };
}
