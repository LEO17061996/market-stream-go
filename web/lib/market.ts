export type Instrument = {
  symbol: string;
  name: string;
  exchange: string;
  unit: string;
  decimals: number;
  tickSize: number;
  contractSize: number;
  priceToUSD: number;
  marginUSD: number;
};

export type Tick = { s: string; p: number; t: number };

export type Quote = {
  symbol: string;
  price: number;
  open: number;
  high: number;
  low: number;
  change: number;
  changePct: number;
  time: number;
};

export type Candle = { time: number; open: number; high: number; low: number; close: number };

export const fmtPrice = (v: number, decimals: number) =>
  v.toLocaleString("vi-VN", { minimumFractionDigits: decimals, maximumFractionDigits: decimals });

export const fmtVND = (v: number) => Math.round(v).toLocaleString("vi-VN") + " đ";

export const fmtTime = (ms: number) =>
  new Date(ms).toLocaleTimeString("vi-VN", { timeZone: "Asia/Ho_Chi_Minh", hour12: false });

export const fmtDateTime = (ms: number) =>
  new Date(ms).toLocaleString("vi-VN", { timeZone: "Asia/Ho_Chi_Minh", hour12: false, day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });

/** Cập nhật dòng bảng giá bằng tick, cùng công thức với máy chủ (internal/market/quote.go). */
export function applyTick(q: Quote | undefined, t: Tick): Quote {
  const base = q ?? { symbol: t.s, price: t.p, open: t.p, high: t.p, low: t.p, change: 0, changePct: 0, time: t.t };
  const change = t.p - base.open;
  return {
    ...base,
    price: t.p,
    high: Math.max(base.high, t.p),
    low: Math.min(base.low, t.p),
    change,
    changePct: base.open ? Math.round((change / base.open) * 10000) / 100 : 0,
    time: t.t,
  };
}

/** Gộp tick vào nến 1 phút, cùng quy tắc với máy chủ: nến mới mở bằng giá đóng của nến trước. */
export function applyTickToCandle(cur: Candle | undefined, t: Tick, frameSec = 60): Candle {
  const bucket = Math.floor(t.t / 1000 / frameSec) * frameSec;
  if (!cur || bucket > cur.time) {
    const open = cur ? cur.close : t.p;
    return { time: bucket, open, high: Math.max(open, t.p), low: Math.min(open, t.p), close: t.p };
  }
  if (bucket < cur.time) return cur; // tick đến muộn
  return { ...cur, high: Math.max(cur.high, t.p), low: Math.min(cur.low, t.p), close: t.p };
}

export async function api<T>(path: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const { json, ...rest } = init ?? {};
  const res = await fetch(path, {
    credentials: "same-origin",
    ...rest,
    headers: { Accept: "application/json", ...(json !== undefined ? { "Content-Type": "application/json" } : {}), ...rest.headers },
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw Object.assign(new Error(data.error || `Lỗi ${res.status}`), { status: res.status });
  return data as T;
}
