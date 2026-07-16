"use client";

import { useState, useEffect, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";

// ─────────────────────────────────────────────────────────────────────────────
// TYPES
// ─────────────────────────────────────────────────────────────────────────────
interface Stats {
  active_drop_id: string;
  active_drop_name: string;
  active_drop_starts_at: string | null;
  active_drop_ends_at: string | null;
  stock_remaining: number;
  total_stock: number;
  total_orders: number;
  pending_orders: number;
  expired_orders: number;
  completed_orders: number;
  revenue_cents: number;
}

interface Drop {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  total_stock: number;
  starts_at: string;
  ends_at: string;
  stock_remaining: number;
  completed_orders: number;
}

interface Order {
  id: string;
  drop_id: string;
  drop_name: string;
  user_id: string;
  status: "pending" | "completed" | "expired";
  expires_at: string;
  created_at: string;
}

type Tab = "overview" | "drops" | "orders";

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────
const fmtPrice = (cents: number) => `$${(cents / 100).toFixed(2)}`;
const fmtDate  = (iso: string)   => new Date(iso).toLocaleString("en-GB", { dateStyle: "short", timeStyle: "short" });
const fmtPhase = (drop: Drop) => {
  const now = Date.now();
  if (now < new Date(drop.starts_at).getTime()) return { label: "PRE-DROP",  color: "text-yellow-400 border-yellow-700" };
  if (now > new Date(drop.ends_at).getTime())   return { label: "ENDED",     color: "text-zinc-500 border-zinc-700" };
  if (drop.stock_remaining === 0)               return { label: "SOLD OUT",  color: "text-red-400 border-red-800" };
  return                                               { label: "LIVE",      color: "text-green-400 border-green-800" };
};

// ─────────────────────────────────────────────────────────────────────────────
// API CLIENT
// ─────────────────────────────────────────────────────────────────────────────
function useApi(token: string | null, onUnauthorized: () => void) {
  const headers = useCallback((extra?: Record<string, string>) => ({
    "Content-Type": "application/json",
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...extra,
  }), [token]);

  const get  = useCallback((path: string) =>
    fetch(path, { headers: headers() }).then(r => {
      if (r.status === 401) { onUnauthorized(); return {}; }
      return r.json();
    }), [headers, onUnauthorized]);

  const post = useCallback((path: string, body: unknown) =>
    fetch(path, { method: "POST", headers: headers(), body: JSON.stringify(body) }).then(r => {
      if (r.status === 401) { onUnauthorized(); return {}; }
      return r.json();
    }), [headers, onUnauthorized]);

  const patch = useCallback((path: string, body: unknown) =>
    fetch(path, { method: "PATCH", headers: headers(), body: JSON.stringify(body) }).then(r => {
      if (r.status === 401) { onUnauthorized(); return {}; }
      return r.json();
    }), [headers, onUnauthorized]);

  return { get, post, patch };
}

// ─────────────────────────────────────────────────────────────────────────────
// LOGIN SCREEN
// ─────────────────────────────────────────────────────────────────────────────
function LoginScreen({ onLogin }: { onLogin: (token: string) => void }) {
  const [password, setPassword] = useState("");
  const [error,    setError]    = useState("");
  const [loading,  setLoading]  = useState(false);

  const submit = async () => {
    if (!password) return;
    setLoading(true); setError("");
    try {
      const res = await fetch("/api/admin/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      const data = await res.json();
      if (!res.ok) { setError(data.error ?? "Wrong password"); setLoading(false); return; }
      onLogin(data.token);
    } catch {
      setError("Network error"); setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-black flex items-center justify-center"
      style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>
      <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
        className="w-full max-w-sm space-y-6 px-6">
        <div>
          <p className="text-xs font-mono tracking-widest uppercase text-zinc-600 mb-2">DOOMSDAY™</p>
          <h1 className="text-3xl font-black text-white uppercase"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>Admin Access</h1>
        </div>
        <div className="space-y-3">
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            placeholder="Enter password"
            className="w-full h-12 bg-zinc-950 border border-zinc-700 text-white text-sm font-mono px-4 focus:outline-none focus:border-zinc-400 placeholder:text-zinc-600"
          />
          <AnimatePresence>
            {error && (
              <motion.p initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }}
                exit={{ opacity: 0, height: 0 }}
                className="text-xs font-mono text-red-400 tracking-widest uppercase">
                ✕ {error}
              </motion.p>
            )}
          </AnimatePresence>
          <button onClick={submit} disabled={loading}
            className="w-full h-12 bg-white text-black text-sm font-mono font-bold tracking-widest uppercase hover:bg-zinc-200 transition-colors disabled:opacity-50">
            {loading ? "Authenticating…" : "Enter →"}
          </button>
        </div>
        <p className="text-xs font-mono text-zinc-700 text-center">Default password: doomsday-admin</p>
      </motion.div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// STAT CARD
// ─────────────────────────────────────────────────────────────────────────────
function StatCard({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div className="border border-zinc-800 bg-zinc-950 p-5 space-y-1">
      <p className="text-xs font-mono tracking-widest uppercase text-zinc-500">{label}</p>
      <p className="text-3xl font-black text-white" style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>{value}</p>
      {sub && <p className="text-xs font-mono text-zinc-600">{sub}</p>}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// OVERVIEW TAB
// ─────────────────────────────────────────────────────────────────────────────
function OverviewTab({ stats, onRefresh }: { stats: Stats | null; onRefresh: () => void }) {
  if (!stats) return <div className="text-xs font-mono text-zinc-600 p-8">Loading…</div>;

  const stockPct = stats.total_stock > 0 ? stats.stock_remaining / stats.total_stock : 0;
  const now = Date.now();
  const phase = stats.active_drop_starts_at && stats.active_drop_ends_at
    ? now < new Date(stats.active_drop_starts_at).getTime() ? "PRE-DROP"
    : now > new Date(stats.active_drop_ends_at).getTime()   ? "ENDED"
    : stats.stock_remaining === 0 ? "SOLD OUT" : "LIVE"
    : "NO ACTIVE DROP";

  return (
    <div className="space-y-6">
      {/* Active drop banner */}
      <div className="border border-zinc-700 bg-zinc-950 p-5">
        <div className="flex items-start justify-between gap-4 flex-wrap">
          <div>
            <p className="text-xs font-mono tracking-widest uppercase text-zinc-500 mb-1">Active Drop</p>
            <p className="text-xl font-black text-white uppercase"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
              {stats.active_drop_name || "—"}
            </p>
            {stats.active_drop_starts_at && (
              <p className="text-xs font-mono text-zinc-500 mt-1">
                {fmtDate(stats.active_drop_starts_at)} → {stats.active_drop_ends_at ? fmtDate(stats.active_drop_ends_at) : "?"}
              </p>
            )}
          </div>
          <div className={`border px-3 py-1 text-xs font-mono tracking-widest uppercase ${
            phase === "LIVE" ? "border-green-800 text-green-400"
            : phase === "PRE-DROP" ? "border-yellow-800 text-yellow-400"
            : "border-zinc-700 text-zinc-500"
          }`}>{phase}</div>
        </div>

        {stats.total_stock > 0 && (
          <div className="mt-4 space-y-2">
            <div className="flex justify-between text-xs font-mono text-zinc-500">
              <span>Stock</span>
              <span>{stats.stock_remaining} / {stats.total_stock}</span>
            </div>
            <div className="h-1 w-full bg-zinc-800">
              <motion.div className="h-full bg-white" initial={{ width: 0 }}
                animate={{ width: `${stockPct * 100}%` }} transition={{ duration: 1 }} />
            </div>
          </div>
        )}
      </div>

      {/* Stat grid */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <StatCard label="Revenue"   value={fmtPrice(stats.revenue_cents)} />
        <StatCard label="Completed" value={stats.completed_orders} sub="orders" />
        <StatCard label="Pending"   value={stats.pending_orders}   sub="reservations" />
        <StatCard label="Expired"   value={stats.expired_orders}   sub="reservations" />
      </div>

      <button onClick={onRefresh}
        className="text-xs font-mono tracking-widest uppercase text-zinc-600 hover:text-zinc-400 transition-colors border border-zinc-800 px-4 py-2">
        ↺ Refresh
      </button>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// DROPS TAB
// ─────────────────────────────────────────────────────────────────────────────
function DropsTab({ drops, token, onRefresh }: { drops: Drop[]; token: string; onRefresh: () => void }) {
  const { post, patch } = useApi(token, () => {});

  // Create drop form
  const [showForm,  setShowForm]  = useState(false);
  const [creating,  setCreating]  = useState(false);
  const [formError, setFormError] = useState("");
  const [form, setForm] = useState({
    name: "", description: "", price_cents: "", total_stock: "",
    starts_at: "", ends_at: "",
  });

  // Timer reset modal
  const [timerDrop,    setTimerDrop]    = useState<Drop | null>(null);
  const [startsIn,     setStartsIn]     = useState("2");
  const [duration,     setDuration]     = useState("30");
  const [timerLoading, setTimerLoading] = useState(false);

  // Stock reset modal
  const [stockDrop,    setStockDrop]    = useState<Drop | null>(null);
  const [newStock,     setNewStock]     = useState("");
  const [stockLoading, setStockLoading] = useState(false);

  const createDrop = async () => {
    setFormError(""); setCreating(true);
    const body = {
      name:        form.name,
      description: form.description,
      price_cents: parseInt(form.price_cents),
      total_stock: parseInt(form.total_stock),
      starts_at:   form.starts_at,
      ends_at:     form.ends_at,
    };
    const res = await post("/api/admin/drops", body) as any;
    setCreating(false);
    if (res?.error) { setFormError(res.error); return; }
    setShowForm(false);
    setForm({ name:"", description:"", price_cents:"", total_stock:"", starts_at:"", ends_at:"" });
    onRefresh();
  };

  const resetTimer = async () => {
    if (!timerDrop) return;
    setTimerLoading(true);
    await patch(`/api/admin/drops/${timerDrop.id}/timer`, {
      starts_in_minutes: parseInt(startsIn),
      duration_minutes:  parseInt(duration),
    });
    setTimerLoading(false); setTimerDrop(null); onRefresh();
  };

  const resetStock = async () => {
    if (!stockDrop) return;
    setStockLoading(true);
    await patch(`/api/admin/drops/${stockDrop.id}/stock`, { stock: parseInt(newStock) });
    setStockLoading(false); setStockDrop(null); onRefresh();
  };

  return (
    <div className="space-y-4">
      {/* Header row */}
      <div className="flex items-center justify-between">
        <p className="text-xs font-mono tracking-widest uppercase text-zinc-500">{drops.length} drops total</p>
        <button onClick={() => setShowForm((v) => !v)}
          className="text-xs font-mono tracking-widest uppercase border border-zinc-700 text-zinc-300 hover:border-zinc-400 hover:text-white transition-colors px-4 py-2">
          {showForm ? "✕ Cancel" : "+ Create Drop"}
        </button>
      </div>

      {/* Create form */}
      <AnimatePresence>
        {showForm && (
          <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }} className="overflow-hidden">
            <div className="border border-zinc-700 bg-zinc-950 p-5 space-y-4">
              <p className="text-xs font-mono tracking-widest uppercase text-zinc-400">New Drop</p>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {[
                  { key: "name",        label: "Name",           placeholder: "WRAITH FIELD JACKET" },
                  { key: "description", label: "Description",    placeholder: "Military-grade waxed Ventile…" },
                  { key: "price_cents", label: "Price (cents)",  placeholder: "66600" },
                  { key: "total_stock", label: "Total Stock",    placeholder: "100" },
                  { key: "starts_at",   label: "Starts At (RFC3339)", placeholder: "2025-06-01T10:00:00Z" },
                  { key: "ends_at",     label: "Ends At (RFC3339)",   placeholder: "2025-06-01T10:30:00Z" },
                ].map(({ key, label, placeholder }) => (
                  <div key={key} className="space-y-1">
                    <label className="text-xs font-mono tracking-widest uppercase text-zinc-600">{label}</label>
                    <input
                      value={form[key as keyof typeof form]}
                      onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))}
                      placeholder={placeholder}
                      className="w-full h-10 bg-black border border-zinc-700 text-white text-xs font-mono px-3 focus:outline-none focus:border-zinc-500 placeholder:text-zinc-700"
                    />
                  </div>
                ))}
              </div>
              {formError && <p className="text-xs font-mono text-red-400">✕ {formError}</p>}
              <button onClick={createDrop} disabled={creating}
                className="h-10 px-6 bg-white text-black text-xs font-mono font-bold tracking-widest uppercase hover:bg-zinc-200 transition-colors disabled:opacity-50">
                {creating ? "Creating…" : "Create Drop"}
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Drops list */}
      <div className="space-y-2">
        {drops.map((d) => {
          const phase = fmtPhase(d);
          return (
            <div key={d.id} className="border border-zinc-800 bg-zinc-950 p-4">
              <div className="flex items-start justify-between gap-4 flex-wrap">
                <div className="space-y-1 min-w-0">
                  <div className="flex items-center gap-3 flex-wrap">
                    <span className="text-sm font-black text-white uppercase"
                      style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>{d.name}</span>
                    <span className={`text-xs font-mono tracking-widest border px-2 py-0.5 ${phase.color}`}>{phase.label}</span>
                  </div>
                  <p className="text-xs font-mono text-zinc-500">
                    {fmtDate(d.starts_at)} → {fmtDate(d.ends_at)}
                  </p>
                  <p className="text-xs font-mono text-zinc-600">
                    {fmtPrice(d.price_cents)} · stock {d.stock_remaining}/{d.total_stock} · {d.completed_orders} completed
                  </p>
                  <p className="text-xs font-mono text-zinc-700 truncate">{d.id}</p>
                </div>
                <div className="flex gap-2 flex-shrink-0">
                  <button onClick={() => { setTimerDrop(d); setStartsIn("2"); setDuration("30"); }}
                    className="text-xs font-mono tracking-widest uppercase border border-zinc-700 text-zinc-400 hover:border-zinc-500 hover:text-white transition-colors px-3 py-1.5">
                    Timer
                  </button>
                  <button onClick={() => { setStockDrop(d); setNewStock(String(d.total_stock)); }}
                    className="text-xs font-mono tracking-widest uppercase border border-zinc-700 text-zinc-400 hover:border-zinc-500 hover:text-white transition-colors px-3 py-1.5">
                    Stock
                  </button>
                </div>
              </div>
            </div>
          );
        })}
        {drops.length === 0 && (
          <p className="text-xs font-mono text-zinc-700 p-4">No drops found.</p>
        )}
      </div>

      {/* Timer modal */}
      <AnimatePresence>
        {timerDrop && (
          <Modal title={`Reset Timer — ${timerDrop.name}`} onClose={() => setTimerDrop(null)}>
            <p className="text-xs font-mono text-zinc-500 mb-4">Reschedule this drop relative to now.</p>
            <div className="grid grid-cols-2 gap-3 mb-4">
              <div className="space-y-1">
                <label className="text-xs font-mono uppercase tracking-widest text-zinc-600">Starts in (min)</label>
                <input value={startsIn} onChange={(e) => setStartsIn(e.target.value)}
                  className="w-full h-10 bg-black border border-zinc-700 text-white text-xs font-mono px-3 focus:outline-none focus:border-zinc-500" />
              </div>
              <div className="space-y-1">
                <label className="text-xs font-mono uppercase tracking-widest text-zinc-600">Duration (min)</label>
                <input value={duration} onChange={(e) => setDuration(e.target.value)}
                  className="w-full h-10 bg-black border border-zinc-700 text-white text-xs font-mono px-3 focus:outline-none focus:border-zinc-500" />
              </div>
            </div>
            <button onClick={resetTimer} disabled={timerLoading}
              className="w-full h-10 bg-white text-black text-xs font-mono font-bold tracking-widest uppercase hover:bg-zinc-200 transition-colors disabled:opacity-50">
              {timerLoading ? "Updating…" : "Apply"}
            </button>
          </Modal>
        )}
      </AnimatePresence>

      {/* Stock modal */}
      <AnimatePresence>
        {stockDrop && (
          <Modal title={`Reset Stock — ${stockDrop.name}`} onClose={() => setStockDrop(null)}>
            <p className="text-xs font-mono text-zinc-500 mb-4">Set available stock and redistribute it across sizes. PostgreSQL and Redis will be synchronized.</p>
            <div className="space-y-1 mb-4">
              <label className="text-xs font-mono uppercase tracking-widest text-zinc-600">New stock value</label>
              <input value={newStock} onChange={(e) => setNewStock(e.target.value)}
                className="w-full h-10 bg-black border border-zinc-700 text-white text-xs font-mono px-3 focus:outline-none focus:border-zinc-500" />
            </div>
            <button onClick={resetStock} disabled={stockLoading}
              className="w-full h-10 bg-white text-black text-xs font-mono font-bold tracking-widest uppercase hover:bg-zinc-200 transition-colors disabled:opacity-50">
              {stockLoading ? "Updating…" : "Apply"}
            </button>
          </Modal>
        )}
      </AnimatePresence>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// ORDERS TAB
// ─────────────────────────────────────────────────────────────────────────────
function OrdersTab({ orders, statusFilter, onStatusChange }: {
  orders: Order[]; statusFilter: string; onStatusChange: (s: string) => void;
}) {
  const STATUS_COLORS: Record<string, string> = {
    completed: "text-green-400 border-green-800",
    pending:   "text-yellow-400 border-yellow-800",
    expired:   "text-zinc-600 border-zinc-700",
  };

  return (
    <div className="space-y-4">
      {/* Filter */}
      <div className="flex items-center gap-2">
        {["", "pending", "completed", "expired"].map((s) => (
          <button key={s} onClick={() => onStatusChange(s)}
            className={`text-xs font-mono tracking-widest uppercase border px-3 py-1.5 transition-colors ${
              statusFilter === s
                ? "border-white text-white"
                : "border-zinc-700 text-zinc-500 hover:border-zinc-500 hover:text-zinc-300"
            }`}>
            {s || "All"}
          </button>
        ))}
        <span className="text-xs font-mono text-zinc-700 ml-2">{orders.length} results</span>
      </div>

      {/* Table */}
      <div className="border border-zinc-800 overflow-x-auto">
        <table className="w-full text-xs font-mono">
          <thead>
            <tr className="border-b border-zinc-800">
              {["Order ID","Drop","User","Status","Created","Expires"].map((h) => (
                <th key={h} className="text-left px-4 py-3 text-zinc-600 tracking-widest uppercase font-normal whitespace-nowrap">{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {orders.map((o, i) => (
              <tr key={o.id} className={`border-b border-zinc-900 ${i % 2 === 0 ? "bg-zinc-950" : "bg-black"}`}>
                <td className="px-4 py-3 text-zinc-400 font-mono">{o.id.slice(0, 8)}…</td>
                <td className="px-4 py-3 text-zinc-300 whitespace-nowrap">{o.drop_name}</td>
                <td className="px-4 py-3 text-zinc-500">{o.user_id.slice(0, 14)}…</td>
                <td className="px-4 py-3">
                  <span className={`border px-2 py-0.5 tracking-widest uppercase ${STATUS_COLORS[o.status] ?? "text-zinc-500 border-zinc-700"}`}>
                    {o.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-zinc-500 whitespace-nowrap">{fmtDate(o.created_at)}</td>
                <td className="px-4 py-3 text-zinc-600 whitespace-nowrap">{fmtDate(o.expires_at)}</td>
              </tr>
            ))}
            {orders.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-8 text-center text-zinc-700">No orders found.</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// MODAL
// ─────────────────────────────────────────────────────────────────────────────
function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
      className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-6"
      onClick={onClose}>
      <motion.div initial={{ scale: 0.95, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} exit={{ scale: 0.95, opacity: 0 }}
        className="w-full max-w-sm border border-zinc-700 bg-zinc-950 p-6"
        onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <p className="text-xs font-mono tracking-widest uppercase text-white">{title}</p>
          <button onClick={onClose} className="text-zinc-500 hover:text-white text-lg leading-none">✕</button>
        </div>
        {children}
      </motion.div>
    </motion.div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOT
// ─────────────────────────────────────────────────────────────────────────────
export default function AdminPage() {
  const [token,       setToken]       = useState<string | null>(null);
  const [tokenLoaded, setTokenLoaded] = useState(false);
  const [tab,   setTab]   = useState<Tab>("overview");

  const [stats,        setStats]        = useState<Stats | null>(null);
  const [drops,        setDrops]        = useState<Drop[]>([]);
  const [orders,       setOrders]       = useState<Order[]>([]);
  const [statusFilter, setStatusFilter] = useState("");

  const logout = useCallback(() => {
    sessionStorage.removeItem("dmsdy_admin_token");
    setToken(null);
  }, []);

  const { get } = useApi(token, logout);

  // Persist token in sessionStorage so refresh doesn't log out
  useEffect(() => {
    const stored = sessionStorage.getItem("dmsdy_admin_token");
    if (stored) setToken(stored);
    setTokenLoaded(true);
  }, []);

  const handleLogin = (t: string) => {
    sessionStorage.setItem("dmsdy_admin_token", t);
    setToken(t);
  };

  const loadStats  = useCallback(() => get("/api/admin/stats").then(d => { if (d && (d as any).active_drop_id !== undefined) setStats(d as Stats); }),  [get]);
  const loadDrops  = useCallback(() => get("/api/admin/drops").then(d  => setDrops(Array.isArray(d)  ? d : [])),  [get]);
  const loadOrders = useCallback(() => {
    const qs = statusFilter ? `?status=${statusFilter}` : "";
    get(`/api/admin/orders${qs}`).then(d => setOrders(Array.isArray(d) ? d : []));
  }, [get, statusFilter]);

  useEffect(() => { if (!tokenLoaded || !token) return; loadStats(); loadDrops(); }, [tokenLoaded, token]);
  useEffect(() => { if (!tokenLoaded || !token) return; loadOrders(); }, [tokenLoaded, token, statusFilter]);

  if (!token) return <LoginScreen onLogin={handleLogin} />;

  return (
    <div className="min-h-screen bg-black text-white"
      style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>

      {/* Header */}
      <header className="border-b border-zinc-800 px-6 py-4 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <span className="text-xl font-black text-white"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "0.04em" }}>
            DOOMSDAY™
          </span>
          <span className="text-zinc-700">|</span>
          <span className="text-xs font-mono tracking-widest uppercase text-zinc-500">Admin</span>
        </div>
        <button onClick={logout}
          className="text-xs font-mono tracking-widest uppercase text-zinc-600 hover:text-zinc-400 transition-colors">
          Log out
        </button>
      </header>

      {/* Tabs */}
      <div className="border-b border-zinc-800 px-6 flex gap-0">
        {(["overview","drops","orders"] as Tab[]).map((t) => (
          <button key={t} onClick={() => setTab(t)}
            className={`text-xs font-mono tracking-widest uppercase px-5 py-3.5 border-b-2 transition-colors ${
              tab === t
                ? "border-white text-white"
                : "border-transparent text-zinc-500 hover:text-zinc-300"
            }`}>
            {t}
          </button>
        ))}
      </div>

      {/* Content */}
      <main className="p-6 max-w-5xl mx-auto">
        <AnimatePresence mode="wait">
          <motion.div key={tab}
            initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.15 }}>
            {tab === "overview" && <OverviewTab stats={stats} onRefresh={loadStats} />}
            {tab === "drops"    && <DropsTab drops={drops} token={token} onRefresh={() => { loadDrops(); loadStats(); }} />}
            {tab === "orders"   && <OrdersTab orders={orders} statusFilter={statusFilter} onStatusChange={setStatusFilter} />}
          </motion.div>
        </AnimatePresence>
      </main>
    </div>
  );
}
