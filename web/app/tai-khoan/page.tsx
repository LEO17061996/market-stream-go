"use client";

import { useCallback, useEffect, useState } from "react";
import { Disclaimer, Header } from "@/components/Shell";
import { api, fmtDateTime, fmtPrice, fmtVND, type Instrument } from "@/lib/market";

type Account = { id: number; code: string; balanceVND: number; pendingWithdrawVND: number; availableVND: number };
type Me = { user: { id: number; username: string; fullName: string }; account: Account };
type Txn = { id: number; kind: "deposit" | "withdraw"; amountVND: number; status: "pending" | "completed" | "rejected"; note: string; createdAt: number };
type Margin = { margin: { lots: number; price: number; notionalUSD: number; marginUSD: number; usdvnd: number; marginVND: number }; maxLots?: number; availableVND?: number };

const STATUS: Record<Txn["status"], string> = { pending: "Chờ duyệt", completed: "Hoàn tất", rejected: "Từ chối" };

export default function AccountPage() {
  const [me, setMe] = useState<Me | null | undefined>(undefined);
  const refresh = useCallback(() => api<Me>("/api/me").then(setMe).catch(() => setMe(null)), []);
  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <>
      <Header />
      <Disclaimer />
      <main className="p-4">
        {me === undefined ? null : me === null ? (
          <Login onDone={refresh} />
        ) : (
          <Dashboard me={me} refresh={refresh} onLogout={() => api("/api/logout", { method: "POST", json: {} }).then(() => setMe(null))} />
        )}
      </main>
    </>
  );
}

function Login({ onDone }: { onDone: () => void }) {
  const [username, setUsername] = useState("demo");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/login", { method: "POST", json: { username, password } });
      onDone();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form onSubmit={submit} className="card mx-auto flex max-w-sm flex-col gap-3">
      <h1 className="text-base font-semibold">Đăng nhập tài khoản ký quỹ</h1>
      <label className="flex flex-col gap-1 text-sm">
        Tên đăng nhập
        <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
      </label>
      <label className="flex flex-col gap-1 text-sm">
        Mật khẩu
        <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
      </label>
      {error && <p className="text-sm text-[var(--down)]">{error}</p>}
      <button className="btn" disabled={busy}>
        Đăng nhập
      </button>
      <p className="text-xs text-[var(--muted)]">
        Tài khoản demo: <code>demo</code> / <code>123456</code>. Phiên lưu trong Redis 24 giờ; sai quá 10 lần trong 10 phút sẽ bị chặn tạm.
      </p>
    </form>
  );
}

function Dashboard({ me, refresh, onLogout }: { me: Me; refresh: () => void; onLogout: () => void }) {
  const a = me.account;
  return (
    <div className="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,420px),1fr))]">
      <section className="card flex flex-col gap-3">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h1 className="text-base font-semibold">
            {me.user.fullName} · {a.code}
          </h1>
          <button onClick={onLogout} className="text-sm text-[var(--muted)] hover:text-[var(--ink)]">
            Đăng xuất
          </button>
        </div>
        <div className="grid gap-2 [grid-template-columns:repeat(auto-fit,minmax(140px,1fr))]">
          <Stat label="Số dư ký quỹ" value={fmtVND(a.balanceVND)} />
          <Stat label="Đang chờ rút" value={fmtVND(a.pendingWithdrawVND)} />
          <Stat label="Khả dụng" value={fmtVND(a.availableVND)} strong />
        </div>
        <MarginCalc />
        <Withdraw available={a.availableVND} onDone={refresh} />
      </section>
      <History key={a.pendingWithdrawVND} />
    </div>
  );
}

function Stat({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className="rounded-lg border border-[var(--line)] px-3 py-2">
      <div className="text-xs text-[var(--muted)]">{label}</div>
      <div className={`tabular-nums ${strong ? "text-lg font-semibold" : ""}`}>{value}</div>
    </div>
  );
}

function MarginCalc() {
  const [instruments, setInstruments] = useState<Instrument[]>([]);
  const [symbol, setSymbol] = useState("GC");
  const [lots, setLots] = useState(1);
  const [res, setRes] = useState<Margin | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api<Instrument[]>("/api/instruments").then((list) => setInstruments(list.filter((i) => i.contractSize > 0)));
  }, []);
  useEffect(() => {
    if (!lots) return;
    const run = () =>
      api<Margin>("/api/margin", { method: "POST", json: { symbol, lots } })
        .then((r) => {
          setRes(r);
          setError("");
        })
        .catch((e) => setError(e.message));
    run();
    const timer = setInterval(run, 3000); // giá đổi liên tục nên ký quỹ quy đổi cũng đổi theo
    return () => clearInterval(timer);
  }, [symbol, lots]);
  const ins = instruments.find((i) => i.symbol === symbol);

  return (
    <div className="rounded-lg border border-[var(--line)] p-3">
      <h2 className="mb-2 text-sm font-semibold">Tính ký quỹ</h2>
      <div className="flex flex-wrap gap-2">
        <select className="input" value={symbol} onChange={(e) => setSymbol(e.target.value)}>
          {instruments.map((i) => (
            <option key={i.symbol} value={i.symbol}>
              {i.symbol} · {i.name}
            </option>
          ))}
        </select>
        <input className="input w-28" type="number" min={1} max={1000} step={1} value={lots} onChange={(e) => setLots(Math.max(0, Math.floor(Number(e.target.value))))} aria-label="Số hợp đồng" />
        <span className="self-center text-sm text-[var(--muted)]">hợp đồng</span>
      </div>
      {error && <p className="mt-2 text-sm text-[var(--down)]">{error}</p>}
      {res && ins && (
        <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-[var(--muted)]">Giá hiện tại</dt>
          <dd className="tabular-nums">
            {fmtPrice(res.margin.price, ins.decimals)} {ins.unit}
          </dd>
          <dt className="text-[var(--muted)]">Giá trị hợp đồng</dt>
          <dd className="tabular-nums">{res.margin.notionalUSD.toLocaleString("vi-VN")} USD</dd>
          <dt className="text-[var(--muted)]">Ký quỹ ban đầu</dt>
          <dd className="tabular-nums">
            {res.margin.marginUSD.toLocaleString("vi-VN")} USD × {res.margin.usdvnd.toLocaleString("vi-VN")} = <b>{fmtVND(res.margin.marginVND)}</b>
          </dd>
          {res.maxLots !== undefined && (
            <>
              <dt className="text-[var(--muted)]">Mở tối đa</dt>
              <dd className="tabular-nums">{res.maxLots} hợp đồng với số dư khả dụng</dd>
            </>
          )}
        </dl>
      )}
    </div>
  );
}

function Withdraw({ available, onDone }: { available: number; onDone: () => void }) {
  const [amount, setAmount] = useState("");
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const v = Number(amount.replace(/\D/g, ""));
      await api("/api/withdrawals", { method: "POST", json: { amountVND: v } });
      setMsg({ ok: true, text: `Đã gửi yêu cầu rút ${fmtVND(v)}, chờ duyệt.` });
      setAmount("");
      onDone();
    } catch (err) {
      setMsg({ ok: false, text: (err as Error).message });
    } finally {
      setBusy(false);
    }
  };
  return (
    <form onSubmit={submit} className="rounded-lg border border-[var(--line)] p-3">
      <h2 className="mb-2 text-sm font-semibold">Yêu cầu rút ký quỹ</h2>
      <div className="flex flex-wrap gap-2">
        <input className="input min-w-0 flex-1" inputMode="numeric" placeholder="Số tiền (đ)" value={amount} onChange={(e) => setAmount(e.target.value)} />
        <button className="btn" disabled={busy || !amount}>
          Gửi yêu cầu
        </button>
      </div>
      <p className="mt-2 text-xs text-[var(--muted)]">Khả dụng {fmtVND(available)}. Máy chủ khoá dòng tài khoản khi kiểm số dư, gửi nhiều yêu cầu cùng lúc cũng không rút quá.</p>
      {msg && <p className={`mt-2 text-sm ${msg.ok ? "text-[var(--up)]" : "text-[var(--down)]"}`}>{msg.text}</p>}
    </form>
  );
}

function History() {
  const [items, setItems] = useState<Txn[]>([]);
  const [next, setNext] = useState(0);
  const [loading, setLoading] = useState(false);
  const load = useCallback(async (before: number) => {
    setLoading(true);
    try {
      const page = await api<{ items: Txn[]; nextBefore: number }>(`/api/transactions?limit=20${before ? `&before=${before}` : ""}`);
      setItems((prev) => (before ? [...prev, ...page.items] : page.items));
      setNext(page.nextBefore);
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    load(0);
  }, [load]);

  return (
    <section className="card">
      <h2 className="mb-2 px-1 text-base font-semibold">Lịch sử nạp / rút ký quỹ</h2>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[480px] text-sm">
          <thead>
            <tr className="text-left text-[var(--muted)]">
              <th className="px-2 py-2 font-medium">Thời gian</th>
              <th className="px-2 py-2 font-medium">Loại</th>
              <th className="px-2 py-2 text-right font-medium">Số tiền</th>
              <th className="px-2 py-2 font-medium">Trạng thái</th>
            </tr>
          </thead>
          <tbody>
            {items.map((t) => (
              <tr key={t.id} className="border-t border-[var(--line)]" title={t.note}>
                <td className="px-2 py-2 tabular-nums text-[var(--muted)]">{fmtDateTime(t.createdAt)}</td>
                <td className="px-2 py-2">{t.kind === "deposit" ? "Nạp" : "Rút"}</td>
                <td className={`px-2 py-2 text-right tabular-nums ${t.kind === "deposit" ? "text-[var(--up)]" : ""}`}>
                  {t.kind === "deposit" ? "+" : "−"}
                  {fmtVND(t.amountVND)}
                </td>
                <td className="px-2 py-2">
                  <span className={t.status === "pending" ? "text-yellow-400" : t.status === "rejected" ? "text-[var(--down)]" : "text-[var(--muted)]"}>{STATUS[t.status]}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {next > 0 && (
        <button onClick={() => load(next)} disabled={loading} className="mt-3 w-full rounded-lg border border-[var(--line)] py-2 text-sm hover:bg-[var(--hover)]">
          {loading ? "Đang tải…" : "Xem thêm"}
        </button>
      )}
      <p className="mt-2 px-1 text-xs text-[var(--muted)]">Phân trang theo khoá (id &lt; mốc), không dùng OFFSET nên trang sau nhanh như trang đầu.</p>
    </section>
  );
}
