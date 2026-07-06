"use client";

import { useSearchParams, useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { useEffect, useState } from "react";

export default function ConfirmationPage() {
  const searchParams = useSearchParams();
  const router       = useRouter();
  const orderID  = searchParams.get("order") ?? "—";
  const itemName = searchParams.get("item")  ?? "WRAITH FIELD JACKET";

  const [show, setShow] = useState(false);
  useEffect(() => { const t = setTimeout(() => setShow(true), 100); return () => clearTimeout(t); }, []);

  const dispatch = new Date(Date.now() + 2 * 24 * 60 * 60 * 1000)
    .toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });

  return (
    <div className="min-h-screen bg-black text-white flex flex-col" style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>
      <style>{`@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;700&display=swap');`}</style>
      <div aria-hidden className="fixed inset-0 pointer-events-none opacity-[0.03]"
        style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`, backgroundSize: "160px 160px" }} />

      <header className="border-b border-zinc-800 px-6 py-4">
        <span className="text-xl font-black" style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "0.04em" }}>DOOMSDAY™</span>
      </header>

      <main className="flex-1 flex flex-col items-center justify-center px-6 py-16 text-center">
        <div className="relative overflow-hidden mb-8">
          <motion.div initial={{ y: "110%", opacity: 0 }} animate={show ? { y: "0%", opacity: 1 } : {}} transition={{ duration: 0.7, ease: [0.16, 1, 0.3, 1] }}>
            <h1 className="text-[18vw] md:text-[12vw] font-black leading-none uppercase text-white"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.02em" }}>SECURED</h1>
          </motion.div>
          <motion.div initial={{ y: "110%", opacity: 0 }} animate={show ? { y: "0%", opacity: [0, 0.3, 0] } : {}} transition={{ duration: 1.2 }} aria-hidden className="absolute inset-0 flex items-center justify-center">
            <span className="text-[18vw] md:text-[12vw] font-black leading-none uppercase text-red-500 select-none"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.02em", transform: "translateX(4px)" }}>SECURED</span>
          </motion.div>
        </div>

        <motion.div initial={{ opacity: 0, y: 20 }} animate={show ? { opacity: 1, y: 0 } : {}} transition={{ delay: 0.4 }} className="space-y-8 max-w-md w-full">
          <div className="border border-zinc-700 bg-zinc-950 p-6 text-left space-y-4">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-xs font-mono tracking-widest uppercase text-zinc-500 mb-1">Order ID</p>
                <p className="text-lg font-black text-white" style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>{orderID.toUpperCase()}</p>
              </div>
              <div className="border border-zinc-700 bg-black px-2.5 py-1">
                <span className="text-xs font-mono tracking-widest uppercase text-zinc-400">Confirmed</span>
              </div>
            </div>
            <div className="h-px bg-zinc-800" />
            <div className="space-y-3">
              {[["Item", decodeURIComponent(itemName)],["Status","Order Confirmed"],["Est. Dispatch", dispatch],["Delivery","DHL Express · Tracked"]].map(([k,v]) => (
                <div key={k} className="flex justify-between items-baseline gap-4">
                  <span className="text-xs font-mono text-zinc-600 tracking-widest uppercase shrink-0">{k}</span>
                  <span className="text-xs font-mono text-zinc-300 text-right">{v}</span>
                </div>
              ))}
            </div>
          </div>

          <p className="text-sm font-mono text-zinc-500 leading-relaxed">
            A confirmation has been dispatched. Your piece ships within 48 hours. Track via the DHL reference in your email.
          </p>

          <div className="flex flex-col sm:flex-row gap-3">
            <button onClick={() => router.push("/drops")}
              className="flex-1 h-12 border border-zinc-700 text-xs tracking-widest uppercase font-mono text-zinc-400 hover:border-zinc-500 hover:text-zinc-300 transition-colors">
              Back to Drop
            </button>
            <button onClick={() => window.print()}
              className="flex-1 h-12 border border-white text-xs tracking-widest uppercase font-mono text-white hover:bg-white hover:text-black transition-colors">
              Save Receipt
            </button>
          </div>
          <p className="text-xs font-mono text-zinc-800 tracking-wide">All sales final · No returns · No restocks</p>
        </motion.div>
      </main>

      <footer className="border-t border-zinc-900 px-6 py-4 flex items-center justify-between">
        <span className="text-xs font-mono tracking-widest uppercase text-zinc-700">© DOOMSDAY MMXXV</span>
        <span className="text-xs font-mono tracking-widest uppercase text-zinc-700">End of Days Sale System v3.0</span>
      </footer>
    </div>
  );
}