"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams, useSearchParams, useRouter } from "next/navigation";
import Image from "next/image";
import { motion, AnimatePresence } from "framer-motion";

type SubmitState = "idle" | "loading" | "success" | "error" | "expired";
interface FormData { email: string; name: string; address: string; }
interface DropInfo { id: string; name: string; price_cents: number; }

function useCountdown(target: Date | null) {
  const calc = useCallback(() => {
    if (!target) return { minutes: 0, seconds: 0 };
    const diff = Math.max(0, target.getTime() - Date.now());
    return { minutes: Math.floor((diff % 3_600_000) / 60_000), seconds: Math.floor((diff % 60_000) / 1_000) };
  }, [target]);
  const [t, setT] = useState(calc);
  useEffect(() => { const id = setInterval(() => setT(calc()), 1_000); return () => clearInterval(id); }, [calc]);
  return t;
}

function Field({ label, value, onChange, placeholder, type = "text" }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string;
}) {
  return (
    <div className="space-y-1.5">
      <label className="block text-xs font-mono tracking-widest uppercase text-zinc-500">{label}</label>
      <input type={type} value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder}
        className="w-full h-11 bg-zinc-950 border border-zinc-700 px-4 text-sm font-mono text-white
                   placeholder-zinc-700 focus:outline-none focus:border-white transition-colors" />
    </div>
  );
}

export default function CheckoutPage() {
  const params       = useParams<{ reservationID: string }>();
  const searchParams = useSearchParams();
  const router       = useRouter();

  const reservationID = params.reservationID;
  const dropID        = searchParams.get("drop") ?? "";
  const expiresAtStr  = searchParams.get("expires") ?? "";
  const expiresAt     = expiresAtStr ? new Date(expiresAtStr) : null;
  const time          = useCountdown(expiresAt);
  const urgent        = time.minutes < 2 && (time.minutes > 0 || time.seconds > 0);
  const expired       = expiresAt ? Date.now() > expiresAt.getTime() : false;

  const [form,        setForm]        = useState<FormData>({ email: "", name: "", address: "" });
  const [drop,        setDrop]        = useState<DropInfo | null>(null);
  const [submitState, setSubmitState] = useState<SubmitState>("idle");
  const [errorMsg,    setErrorMsg]    = useState("");
  const [orderID,     setOrderID]     = useState("");
  const [imgError,    setImgError]    = useState(false);

  const set = (key: keyof FormData) => (v: string) => setForm(f => ({ ...f, [key]: v }));

  // ── Fetch drop info for name + price ──────────────────────────────────────
  useEffect(() => {
    if (!dropID) return;
    fetch(`/api/drops/${dropID}`)
      .then(r => r.json())
      .then(d => { if (d?.id) setDrop(d); })
      .catch(() => {});
  }, [dropID]);

  // ── Pre-fill form: saved checkout data > auth email ───────────────────────
  useEffect(() => {
    const authEmail = localStorage.getItem("dmsdy_user_email") ?? "";
    const saved     = localStorage.getItem(`dmsdy_checkout_${reservationID}`);
    if (saved) {
      try {
        const p = JSON.parse(saved) as Partial<FormData>;
        setForm({ email: p.email ?? authEmail, name: p.name ?? "", address: p.address ?? "" });
        return;
      } catch {}
    }
    if (authEmail) setForm(f => ({ ...f, email: authEmail }));
  }, [reservationID]);

  // ── Persist form on every change ──────────────────────────────────────────
  useEffect(() => {
    if (!reservationID) return;
    localStorage.setItem(`dmsdy_checkout_${reservationID}`, JSON.stringify(form));
  }, [form, reservationID]);

  const isValid = form.email.includes("@") && form.name.trim().length > 0 && form.address.trim().length > 0;

  // ── Submit ─────────────────────────────────────────────────────────────────
  const submit = useCallback(async () => {
    if (!isValid || submitState === "loading" || expired) return;
    setSubmitState("loading");
    const token = localStorage.getItem("dmsdy_user_token") ?? "";
    try {
      const res = await fetch(`/api/checkout/${reservationID}/complete`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ email: form.email, name: form.name, address: form.address }),
      });
      const body = await res.json();
      if (res.ok) {
        localStorage.removeItem(`dmsdy_checkout_${reservationID}`);
        localStorage.removeItem(`dmsdy_reservation_${dropID}`);
        setOrderID(body.order_id);
        setSubmitState("success");
        setTimeout(() => router.push(
          `/confirmation?order=${body.order_id}&item=${encodeURIComponent(drop?.name ?? "")}`
        ), 2_000);
        return;
      }
      if (res.status === 410) { setSubmitState("expired"); return; }
      setErrorMsg(body.error ?? "Something went wrong");
      setSubmitState("error");
    } catch {
      setErrorMsg("Network error — try again");
      setSubmitState("error");
    }
  }, [isValid, submitState, expired, reservationID, form, drop, dropID, router]);

  const price    = drop ? `$${Math.round(drop.price_cents / 100)}` : "—";
  const photoSrc = dropID ? `/product/${dropID}/1.jpg` : null;

  return (
    <div className="min-h-screen bg-black text-white" style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>
      <style>{`@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;700&display=swap');`}</style>
      <div aria-hidden className="fixed inset-0 pointer-events-none opacity-[0.03]"
        style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`, backgroundSize: "160px 160px" }} />

      {/* Header */}
      <header className="border-b border-zinc-800 px-6 py-4 flex items-center justify-between">
        <button onClick={() => router.push("/drops")}
          className="text-xl font-black text-white hover:text-zinc-300 transition-colors"
          style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "0.04em" }}>
          DOOMSDAY™
        </button>
        <div className="flex items-center gap-3">
          <span className="text-xs font-mono text-zinc-500 tracking-widest uppercase">Checkout</span>
          {expiresAt && (
            <div className={`border px-3 py-1.5 flex items-center gap-2 ${urgent ? "border-red-800" : "border-zinc-700"}`}>
              <span className="text-xs font-mono text-zinc-400 tracking-widest uppercase">Expires</span>
              <motion.span
                animate={urgent ? { color: ["#ef4444","#ffffff","#ef4444"] } : {}}
                transition={{ duration: 0.8, repeat: urgent ? Infinity : 0 }}
                className="text-sm font-black tabular-nums text-white"
                style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
                {String(time.minutes).padStart(2,"0")}:{String(time.seconds).padStart(2,"0")}
              </motion.span>
            </div>
          )}
        </div>
      </header>

      {/* Expired overlay */}
      <AnimatePresence>
        {(expired || submitState === "expired") && (
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}
            className="fixed inset-0 z-50 bg-black flex items-center justify-center p-8">
            <div className="text-center space-y-6 max-w-sm">
              <p className="text-6xl font-black text-zinc-800"
                style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>EXPIRED</p>
              <p className="text-sm font-mono text-zinc-500">
                Your reservation timed out. Return to the drop and try again if stock remains.
              </p>
              <button onClick={() => router.push(dropID ? `/drops/${dropID}` : "/drops")}
                className="w-full h-12 border border-zinc-700 text-xs tracking-widest uppercase font-mono
                           text-zinc-400 hover:border-white hover:text-white transition-colors">
                Back to Drop
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Success flash */}
      <AnimatePresence>
        {submitState === "success" && (
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }}
            className="fixed inset-0 z-50 bg-white flex items-center justify-center">
            <motion.div initial={{ scale: 0.8, opacity: 0 }} animate={{ scale: 1, opacity: 1 }}
              transition={{ delay: 0.1 }} className="text-center">
              <p className="text-7xl font-black text-black"
                style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>SECURED</p>
              <p className="text-sm font-mono text-zinc-600 mt-3">Order {orderID} confirmed.</p>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Main layout */}
      <div className="max-w-4xl mx-auto p-6 md:p-10 grid grid-cols-1 md:grid-cols-[1fr_320px] gap-10">

        {/* LEFT — form */}
        <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }} className="space-y-8">

          <div className="space-y-4">
            <h2 className="text-xs tracking-widest uppercase font-mono text-zinc-500 pb-2 border-b border-zinc-900">
              01 — Contact
            </h2>
            <Field label="Email" value={form.email} onChange={set("email")} placeholder="your@email.com" type="email" />
            <Field label="Full Name" value={form.name} onChange={set("name")} placeholder="John Doe" />
          </div>

          <div className="space-y-4">
            <h2 className="text-xs tracking-widest uppercase font-mono text-zinc-500 pb-2 border-b border-zinc-900">
              02 — Shipping Address
            </h2>
            <Field label="Full Address" value={form.address} onChange={set("address")}
              placeholder="Street, City, Country, Postal Code" />
          </div>

          <div className="space-y-4">
            <h2 className="text-xs tracking-widest uppercase font-mono text-zinc-500 pb-2 border-b border-zinc-900">
              03 — Payment
            </h2>
            <div className="border border-zinc-800 bg-zinc-950 p-4 space-y-1">
              <p className="text-xs font-mono text-zinc-500 tracking-widest uppercase">Demo Mode</p>
              <p className="text-xs font-mono text-zinc-600 leading-relaxed">
                This is a portfolio project. No real payment is processed.
              </p>
            </div>
          </div>

          <div className="space-y-3">
            <motion.button
              onClick={submit}
              disabled={!isValid || submitState === "loading" || expired}
              whileTap={isValid && !expired ? { scale: 0.985 } : {}}
              className={[
                "group relative w-full h-16 overflow-hidden text-sm tracking-widest uppercase font-mono font-bold border transition-colors",
                isValid && !expired && submitState !== "loading"
                  ? "bg-transparent text-white border-white cursor-pointer"
                  : "bg-transparent text-zinc-600 border-zinc-700 cursor-not-allowed",
              ].join(" ")}
            >
              {isValid && !expired && submitState !== "loading" && (
                <motion.span aria-hidden className="absolute inset-0 bg-white origin-left"
                  initial={{ scaleX: 0 }} whileHover={{ scaleX: 1 }}
                  transition={{ duration: 0.22, ease: [0.16, 1, 0.3, 1] }} />
              )}
              <span className="relative z-10 flex items-center justify-center gap-3 group-hover:text-black transition-colors">
                {submitState === "loading" && (
                  <motion.span animate={{ rotate: 360 }} transition={{ duration: 0.9, repeat: Infinity, ease: "linear" }}
                    className="w-4 h-4 border-2 border-zinc-400 border-t-transparent rounded-full" />
                )}
                {submitState === "loading" ? "Processing…" : `Complete Order — ${price}`}
              </span>
            </motion.button>

            <AnimatePresence>
              {submitState === "error" && (
                <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }} className="border border-red-900 bg-red-950/20 p-4">
                  <p className="text-xs font-mono text-red-400">✕ {errorMsg}</p>
                </motion.div>
              )}
            </AnimatePresence>

            <p className="text-xs font-mono text-zinc-700 text-center tracking-wide">
              Portfolio demo · No real charges · All sales final
            </p>
          </div>
        </motion.div>

        {/* RIGHT — order summary */}
        <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.1 }} className="space-y-4">
          <h2 className="text-xs tracking-widest uppercase font-mono text-zinc-500 pb-2 border-b border-zinc-900">
            Order Summary
          </h2>

          <div className="border border-zinc-800 p-4 flex gap-4">
            {/* Product photo */}
            <div className="w-16 h-20 bg-zinc-900 border border-zinc-800 shrink-0 overflow-hidden">
              {photoSrc && !imgError ? (
                <Image src={photoSrc} alt={drop?.name ?? "Product"} width={64} height={80}
                  className="w-full h-full object-cover" onError={() => setImgError(true)} />
              ) : (
                <div className="w-full h-full flex items-center justify-center">
                  <svg viewBox="0 0 100 130" className="w-10 opacity-30" fill="none">
                    <path d="M25 0 L10 25 L0 25 L0 80 L15 80 L15 130 L85 130 L85 80 L100 80 L100 25 L90 25 L75 0 Z" fill="white"/>
                    <path d="M35 0 L50 20 L65 0" stroke="white" strokeWidth="2" fill="none"/>
                    <line x1="50" y1="20" x2="50" y2="130" stroke="white" strokeWidth="1.5"/>
                  </svg>
                </div>
              )}
            </div>

            <div className="flex-1 min-w-0">
              <p className="text-xs font-mono tracking-widest uppercase text-zinc-500">SS/25</p>
              <p className="text-sm font-black text-white mt-0.5 leading-tight"
                style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
                {drop?.name ?? <span className="text-zinc-600 text-xs normal-case font-normal">Loading…</span>}
              </p>
              <p className="text-xs font-mono text-zinc-500 mt-1">Qty: 1</p>
            </div>
            <p className="text-sm font-black text-white shrink-0"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>{price}</p>
          </div>

          <div className="space-y-2 border-t border-zinc-800 pt-4">
            {[["Subtotal", price], ["Shipping", "Free"], ["Tax", "Included"]].map(([k, v]) => (
              <div key={k} className="flex justify-between">
                <span className="text-xs font-mono text-zinc-500">{k}</span>
                <span className="text-xs font-mono text-zinc-400">{v}</span>
              </div>
            ))}
            <div className="flex justify-between border-t border-zinc-800 pt-3 mt-3">
              <span className="text-xs font-mono text-white tracking-widest uppercase font-bold">Total</span>
              <span className="text-sm font-black text-white"
                style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>{price}</span>
            </div>
          </div>

          <div className="border border-zinc-900 bg-zinc-950 p-3">
            <p className="text-xs font-mono text-zinc-600">Reservation ID</p>
            <p className="text-xs font-mono text-zinc-500 mt-0.5 truncate">{reservationID}</p>
          </div>

          <div className="space-y-1.5">
            {["No restocks, ever", "Ships within 48 hours", "Tracked worldwide"].map(t => (
              <div key={t} className="flex items-center gap-2">
                <div className="w-1 h-1 bg-zinc-600 rounded-full shrink-0" />
                <span className="text-xs font-mono text-zinc-600">{t}</span>
              </div>
            ))}
          </div>
        </motion.div>
      </div>
    </div>
  );
}