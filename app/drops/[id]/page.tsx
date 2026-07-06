"use client";

import { useState, useEffect, useCallback, useRef, use } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
import {
  motion, AnimatePresence,
  useMotionValue, useTransform, animate,
} from "framer-motion";
import AuthModal from "@/components/AuthModal";

// ─────────────────────────────────────────────────────────────────────────────
// TYPES
// ─────────────────────────────────────────────────────────────────────────────
type DropPhase = "pre" | "live" | "sold_out" | "ended";

interface SizeInfo {
  label: string;
  stock: number;
}

interface DropData {
  id: string; name: string; description: string;
  price_cents: number; total_stock: number;
  starts_at: string; ends_at: string;
  stock_remaining: number; phase: DropPhase;
  sizes: SizeInfo[];
}

interface TimeLeft { days: number; hours: number; minutes: number; seconds: number; }

type ReserveState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "success"; reservationID: string; expiresAt: string }
  | { status: "error"; message: string; retryable: boolean };

type WaitlistState =
  | { status: "idle" } | { status: "loading" }
  | { status: "joined"; position: number } | { status: "error" };

// ─────────────────────────────────────────────────────────────────────────────
// PHOTO CONFIG — put images in /public/product/1.jpg, 2.jpg …
// ─────────────────────────────────────────────────────────────────────────────
// Photos are now resolved per drop via getPhotos() in the root component

// ─────────────────────────────────────────────────────────────────────────────
// GLOBAL STYLES
// ─────────────────────────────────────────────────────────────────────────────
const GLOBAL_CSS = `
  @import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;700&display=swap');
  * { cursor: none !important; }
  .crt-overlay {
    pointer-events: none; position: fixed; inset: 0; z-index: 9990;
  }
  .crt-overlay::before {
    content: ''; position: absolute; inset: 0;
    background: repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(255,255,255,0.04) 3px);
  }
  .crt-overlay::after {
    content: ''; position: absolute; inset: 0;
    background: radial-gradient(ellipse at center, transparent 58%, rgba(0,0,0,0.6) 100%);
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// LETTER SCRAMBLE
// ─────────────────────────────────────────────────────────────────────────────
const SCRAMBLE_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ01#@";

function useScramble(original: string) {
  const [display, setDisplay]   = useState(original);
  const frameRef   = useRef<ReturnType<typeof setInterval> | undefined>(undefined);
  const runningRef = useRef(false);

  const scramble = useCallback(() => {
    if (runningRef.current) return;
    const chars = original.split("");
    const totalFrames = original.replace(/ /g, "").length * 3;
    const scrambleSet = new Set(
      chars.map((ch, i) => ch !== " " ? i : -1)
        .filter((i) => i !== -1)
        .filter(() => Math.random() < 0.45)
    );
    let iteration = 0;
    runningRef.current = true;
    frameRef.current = setInterval(() => {
      setDisplay(
        chars.map((ch, i) => {
          if (ch === " " || !scrambleSet.has(i)) return original[i];
          const resolveAt = Math.floor((i / chars.length) * totalFrames * 0.75);
          if (iteration > resolveAt) return original[i];
          return SCRAMBLE_CHARS[Math.floor(Math.random() * SCRAMBLE_CHARS.length)];
        }).join("")
      );
      iteration++;
      if (iteration > totalFrames) {
        clearInterval(frameRef.current);
        setDisplay(original);
        runningRef.current = false;
      }
    }, 30);
  }, [original]);

  useEffect(() => () => clearInterval(frameRef.current), []);
  return { display, scramble };
}

function ScrambleText({ text, className, style, tag = "span" }: {
  text: string; className?: string; style?: React.CSSProperties; tag?: "span" | "h1";
}) {
  const { display, scramble } = useScramble(text);
  const Tag = tag as keyof JSX.IntrinsicElements;
  return (
    <Tag className={className} style={style} onMouseEnter={scramble}>{display}</Tag>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// PHOTO VIEWER — filmstrip + cursor-follow zoom
// ─────────────────────────────────────────────────────────────────────────────
function PhotoViewer({ photos, phase, price, edition }: {
  photos: string[]; phase: DropPhase; price: string; edition: string;
}) {
  const PHOTOS = photos;
  const [active, setActive] = useState(0);
  const [zoomed, setZoomed] = useState(false);
  // origin as % within the image area — drives transform-origin on the img
  const [origin, setOrigin] = useState({ x: 50, y: 50 });
  const areaRef = useRef<HTMLDivElement>(null);

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = areaRef.current?.getBoundingClientRect();
    if (!rect) return;
    setOrigin({
      x: ((e.clientX - rect.left)  / rect.width)  * 100,
      y: ((e.clientY - rect.top)   / rect.height) * 100,
    });
  };

  return (
    <div className="relative w-full h-full flex flex-col bg-zinc-950 overflow-hidden">

      {/* ── Main photo area ─────────────────────────────────────────── */}
      <div
        ref={areaRef}
        className="relative flex-1 overflow-hidden"
        onMouseEnter={() => setZoomed(true)}
        onMouseLeave={() => { setZoomed(false); setOrigin({ x: 50, y: 50 }); }}
        onMouseMove={handleMouseMove}
      >
        {/* Blurred background duplicate — same src, full cover, darkened */}
        {PHOTOS.map((src, i) => (
          <div key={`bg-${src}`}
            className="absolute inset-0 transition-opacity duration-300"
            style={{ opacity: i === active ? 1 : 0 }}>
            <Image
              src={src}
              alt=""
              aria-hidden
              fill
              className="object-cover object-center scale-110"
              style={{ filter: "blur(24px) brightness(0.25) saturate(0.6)" }}
              sizes="55vw"
            />
          </div>
        ))}

        {/* Ghost number */}
        <div aria-hidden className="absolute inset-0 flex items-center justify-center select-none pointer-events-none z-[1]">
          <span className="text-[40vw] lg:text-[22vw] font-black leading-none mix-blend-overlay text-white/10"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
            {String(active + 1).padStart(2, "0")}
          </span>
        </div>

        {/* Grid overlay */}
        <div className="absolute inset-0 opacity-[0.03] pointer-events-none z-[2]"
          style={{ backgroundImage: "linear-gradient(rgba(255,255,255,1) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,1) 1px,transparent 1px)", backgroundSize: "48px 48px" }} />

        {/* Main photos — object-contain, zoom on the img itself via transform-origin */}
        <div className="absolute inset-0 z-[3] flex items-center justify-center">
          {PHOTOS.map((src, i) => (
            <div key={src}
              className="absolute inset-0"
              style={{ opacity: i === active ? 1 : 0 }}>
              <Image
                src={src}
                alt={`Product photo ${i + 1}`}
                fill
                className="object-contain object-center"
                style={{
                  transformOrigin: `${origin.x}% ${origin.y}%`,
                  transform: zoomed ? "scale(2.4)" : "scale(1)",
                  transition: zoomed
                    ? "transform 0.15s ease-out"
                    : "transform 0.25s ease-out",
                  willChange: "transform",
                }}
                sizes="(max-width: 1024px) 100vw, 55vw"
                priority={i === 0}
                draggable={false}
              />
            </div>
          ))}
        </div>

        {/* Corner marks */}
        {(["top-0 left-0 border-t border-l","top-0 right-0 border-t border-r",
           "bottom-0 left-0 border-b border-l","bottom-0 right-0 border-b border-r"] as const).map((cls) => (
          <div key={cls} className={`absolute w-6 h-6 ${cls} border-white/20 z-[4]`} />
        ))}

        {/* Status pill */}
        <div className="absolute top-5 left-5 z-[5] flex items-center gap-2 border border-zinc-600 bg-black/70 px-3 py-1.5 backdrop-blur-sm">
          <motion.div
            animate={{ opacity: phase === "live" ? [1, 0.15, 1] : 1 }}
            transition={{ duration: 1.2, repeat: phase === "live" ? Infinity : 0 }}
            className={`w-2 h-2 rounded-full ${phase === "live" ? "bg-red-500" : phase === "pre" ? "bg-yellow-400" : "bg-zinc-600"}`}
          />
          <span className="text-xs font-mono tracking-widest uppercase text-zinc-300">
            {phase === "pre" ? "Pre-Drop" : phase === "live" ? "Live" : "Closed"}
          </span>
        </div>

        {/* Zoom cursor hint */}
        <AnimatePresence>
          {!zoomed && (
            <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0, transition: { duration: 0.1 } }}
              transition={{ delay: 0.5 }}
              className="absolute bottom-5 right-5 z-[5] flex items-center gap-2 pointer-events-none">
              <span className="text-xs font-mono tracking-widest uppercase text-zinc-500">Hover to inspect</span>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Price stamp */}
        <motion.div
          initial={{ opacity: 0, rotate: -8, scale: 0.8 }}
          animate={{ opacity: 1, rotate: -8, scale: 1 }}
          transition={{ delay: 0.7 }}
          className="absolute bottom-10 right-8 z-[5] border-2 border-white px-4 py-2 bg-black pointer-events-none">
          <span className="text-2xl font-black text-white"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>{price}</span>
        </motion.div>
      </div>

      {/* ── Filmstrip ──────────────────────────────────────────────── */}
      <div className="flex-shrink-0 flex items-center gap-2 px-4 py-3 border-t border-zinc-800 bg-black z-10">
        <span className="text-xs font-mono text-zinc-600 w-10 flex-shrink-0 tabular-nums">
          {String(active + 1).padStart(2,"0")}&nbsp;/&nbsp;{String(PHOTOS.length).padStart(2,"0")}
        </span>
        <div className="flex gap-2 flex-1">
          {PHOTOS.map((src, i) => (
            <button key={src} onClick={() => setActive(i)}
              className={`relative flex-shrink-0 h-14 overflow-hidden border transition-all duration-100 ${
                i === active
                  ? "border-white w-20 opacity-100"
                  : "border-zinc-700 w-14 opacity-40 hover:opacity-70 hover:border-zinc-500"
              }`}>
              <Image src={src} alt={`Thumb ${i + 1}`} fill
                className="object-cover object-center" sizes="80px" />
            </button>
          ))}
        </div>
        <span className="hidden md:block text-xs font-mono tracking-widest uppercase text-zinc-700 flex-shrink-0">
          {edition}
        </span>
      </div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// SIZE SELECTOR
// ─────────────────────────────────────────────────────────────────────────────
function SizeSelector({ sizes, selected, onSelect }: {
  sizes: SizeInfo[]; selected: string | null; onSelect: (s: string) => void;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-mono tracking-widest uppercase text-zinc-400">Size</span>
        {!selected && (
          <motion.span
            initial={{ opacity: 0 }} animate={{ opacity: 1 }}
            className="text-xs font-mono text-zinc-600">
            Select a size to continue
          </motion.span>
        )}
        {selected && (
          <span className="text-xs font-mono text-white tracking-widest">{selected}</span>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {sizes.map(({ label, stock }) => {
          const soldOut  = stock === 0;
          const isActive = selected === label;
          const low      = stock > 0 && stock <= 3;

          return (
            <button
              key={label}
              onClick={() => !soldOut && onSelect(label)}
              disabled={soldOut}
              className={[
                "relative h-10 min-w-[3rem] px-3 text-xs font-mono tracking-widest uppercase border transition-all duration-100",
                soldOut
                  ? "border-zinc-800 text-zinc-700 cursor-not-allowed line-through"
                  : isActive
                    ? "border-white bg-white text-black"
                    : "border-zinc-700 text-zinc-300 hover:border-zinc-400 hover:text-white",
              ].join(" ")}
            >
              {label}
              {/* Low stock dot */}
              {low && !soldOut && (
                <span className="absolute -top-1 -right-1 w-2 h-2 rounded-full bg-red-500" />
              )}
              {/* Sold out slash */}
              {soldOut && (
                <span aria-hidden className="absolute inset-0 flex items-center justify-center pointer-events-none">
                  <span className="block w-full h-px bg-zinc-700 rotate-[20deg]" />
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Low stock warning */}
      <AnimatePresence>
        {selected && sizes.find(s => s.label === selected)?.stock === 1 && (
          <motion.p initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}
            className="text-xs font-mono text-red-400 tracking-widest uppercase">
            ⚠ Last unit in {selected}
          </motion.p>
        )}
      </AnimatePresence>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// BOOT SEQUENCE
// ─────────────────────────────────────────────────────────────────────────────
const BOOT_LINES = [
  { text: "DOOMSDAY™ DROP SYSTEM v3.0",    delay: 0 },
  { text: "INITIALIZING SECURE CHANNEL…",  delay: 200 },
  { text: "CONNECTING TO INVENTORY NODE…", delay: 400 },
  { text: "AUTHENTICATING SESSION…",       delay: 620 },
  { text: "LOADING DROP #001…",            delay: 840 },
  { text: "STOCK VERIFIED.",               delay: 1040 },
  { text: "ALL SYSTEMS NOMINAL.",          delay: 1200 },
  { text: "DROP LOADED.",                  delay: 1360 },
];

function BootSequence({ onDone }: { onDone: () => void }) {
  const [visible, setVisible] = useState<number[]>([]);
  const [exiting, setExiting] = useState(false);
  useEffect(() => {
    BOOT_LINES.forEach((line, i) =>
      setTimeout(() => setVisible((v) => [...v, i]), line.delay + 200)
    );
    setTimeout(() => {
      setExiting(true);
      setTimeout(onDone, 450);
    }, BOOT_LINES[BOOT_LINES.length - 1].delay + 650);
  }, [onDone]);

  return (
    <motion.div animate={exiting ? { opacity: 0 } : { opacity: 1 }} transition={{ duration: 0.4 }}
      className="fixed inset-0 z-50 bg-black flex flex-col justify-center px-8 md:px-16"
      style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>
      <div className="space-y-2 max-w-lg">
        {BOOT_LINES.map((line, i) => (
          <motion.div key={i}
            initial={{ opacity: 0, x: -8 }}
            animate={visible.includes(i) ? { opacity: 1, x: 0 } : {}}
            transition={{ duration: 0.18 }}
            className={`text-xs md:text-sm font-mono tracking-widest ${
              i === BOOT_LINES.length - 1 ? "text-white font-bold" : "text-zinc-500"
            }`}>
            {i < BOOT_LINES.length - 1 && <span className="text-zinc-700 mr-3">&gt;</span>}
            {line.text}
            {i === visible.length - 1 && i < BOOT_LINES.length - 1 && (
              <motion.span animate={{ opacity: [1, 0, 1] }} transition={{ duration: 0.6, repeat: Infinity }}
                className="ml-1 inline-block w-2 h-3 bg-zinc-500 align-middle" />
            )}
          </motion.div>
        ))}
      </div>
      <div className="absolute bottom-8 left-8 right-8">
        <div className="h-px w-full bg-zinc-900 overflow-hidden">
          <motion.div className="h-full bg-white" initial={{ width: "0%" }} animate={{ width: "100%" }}
            transition={{ duration: (BOOT_LINES[BOOT_LINES.length - 1].delay + 400) / 1000, ease: "linear" }} />
        </div>
        <div className="flex justify-between mt-2">
          <span className="text-xs font-mono text-zinc-700">DOOMSDAY™ SECURE DROP</span>
          <span className="text-xs font-mono text-zinc-700">SS/25</span>
        </div>
      </div>
    </motion.div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// CUSTOM CURSOR
// ─────────────────────────────────────────────────────────────────────────────
function CustomCursor() {
  const cx = useMotionValue(-100); const cy = useMotionValue(-100);
  const tx = useMotionValue(-100); const ty = useMotionValue(-100);
  const [clicked, setClicked] = useState(false);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      setVisible(true);
      cx.set(e.clientX); cy.set(e.clientY);
      animate(tx, e.clientX, { duration: 0.1, ease: "easeOut" });
      animate(ty, e.clientY, { duration: 0.1, ease: "easeOut" });
    };
    const onDown  = () => { setClicked(true); setTimeout(() => setClicked(false), 120); };
    const onLeave = () => setVisible(false);
    const onEnter = () => setVisible(true);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mousedown", onDown);
    document.documentElement.addEventListener("mouseleave", onLeave);
    document.documentElement.addEventListener("mouseenter", onEnter);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mousedown", onDown);
      document.documentElement.removeEventListener("mouseleave", onLeave);
      document.documentElement.removeEventListener("mouseenter", onEnter);
    };
  }, [cx, cy, tx, ty]);

  if (!visible) return null;
  return (
    <>
      <motion.div className="fixed top-0 left-0 pointer-events-none z-[9998]"
        style={{ x: tx, y: ty, translateX: "-50%", translateY: "-50%" }}>
        <div className="w-7 h-7 rounded-full border border-white/20" />
      </motion.div>
      <motion.div className="fixed top-0 left-0 pointer-events-none z-[9999]"
        style={{ x: cx, y: cy, translateX: "-50%", translateY: "-50%" }}>
        <motion.div animate={clicked ? { scale: 0.55 } : { scale: 1 }} transition={{ duration: 0.08 }}
          className="relative w-[18px] h-[18px]">
          <div className="absolute top-1/2 left-0 right-0 h-px bg-white -translate-y-1/2" />
          <div className="absolute left-1/2 top-0 bottom-0 w-px bg-white -translate-x-1/2" />
          <div className="absolute top-1/2 left-1/2 w-[3px] h-[3px] bg-red-500 rounded-full -translate-x-1/2 -translate-y-1/2" />
        </motion.div>
      </motion.div>
    </>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// SOLD OUT FLASH
// ─────────────────────────────────────────────────────────────────────────────
function SoldOutFlash({ show }: { show: boolean }) {
  return (
    <AnimatePresence>
      {show && (
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: [0, 1, 1, 0] }}
          transition={{ duration: 1.1, times: [0, 0.08, 0.72, 1] }}
          className="fixed inset-0 z-40 bg-black flex items-center justify-center pointer-events-none">
          <motion.p initial={{ scale: 0.88 }} animate={{ scale: [0.88, 1.04, 1] }} transition={{ duration: 0.25 }}
            className="text-[14vw] font-black text-white leading-none select-none"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.02em" }}>
            SOLD OUT
          </motion.p>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// HOOKS
// ─────────────────────────────────────────────────────────────────────────────
function useCountdown(target: Date | null): TimeLeft {
  const calc = useCallback(() => {
    if (!target) return { days: 0, hours: 0, minutes: 0, seconds: 0 };
    const diff = Math.max(0, target.getTime() - Date.now());
    return {
      days:    Math.floor(diff / 86_400_000),
      hours:   Math.floor((diff % 86_400_000) / 3_600_000),
      minutes: Math.floor((diff % 3_600_000)  / 60_000),
      seconds: Math.floor((diff % 60_000)     / 1_000),
    };
  }, [target]);
  const [t, setT] = useState<TimeLeft>(calc);
  useEffect(() => { const id = setInterval(() => setT(calc()), 1_000); return () => clearInterval(id); }, [calc]);
  return t;
}

function useAuth() {
  const [token,    setToken]    = useState<string | null>(null);
  const [userToken, setUserToken] = useState<string | null>(null);
  const [email,    setEmail]    = useState<string | null>(null);
  const [isUser,   setIsUser]   = useState(false);

  useEffect(() => {
    // Check for verified user token first
    const ut = localStorage.getItem("dmsdy_user_token");
    const ue = localStorage.getItem("dmsdy_user_email");
    if (ut && ue) {
      setUserToken(ut);
      setToken(ut);
      setEmail(ue);
      setIsUser(true);
      return;
    }
    // Fall back to guest token for browsing
    const stored = localStorage.getItem("dmsdy_token");
    const uid    = localStorage.getItem("dmsdy_uid");
    if (stored && uid) { setToken(stored); return; }
    fetch("/api/auth/guest")
      .then((r) => r.json())
      .then(({ token: t, user_id: u }) => {
        localStorage.setItem("dmsdy_token", t);
        localStorage.setItem("dmsdy_uid",   u);
        setToken(t);
      }).catch(() => {});
  }, []);

  const handleVerified = useCallback((t: string, e: string) => {
    localStorage.setItem("dmsdy_user_token", t);
    localStorage.setItem("dmsdy_user_email", e);
    setUserToken(t);
    setToken(t);
    setEmail(e);
    setIsUser(true);
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem("dmsdy_user_token");
    localStorage.removeItem("dmsdy_user_email");
    setUserToken(null);
    setEmail(null);
    setIsUser(false);
    // Restore guest token
    const stored = localStorage.getItem("dmsdy_token");
    if (stored) setToken(stored);
  }, []);

  return { token, isUser, email, handleVerified, logout };
}


function useDropData(dropID: string) {
  const [data,    setData]    = useState<DropData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error,   setError]   = useState(false);
  useEffect(() => {
    fetch(`/api/drops/${dropID}`)
      .then((r) => { if (!r.ok) throw new Error(String(r.status)); return r.json(); })
      .then((d: DropData) => { setData(d); setLoading(false); })
      .catch(() => { setError(true); setLoading(false); });
  }, [dropID]);
  return { data, loading, error };
}

function useSSEStock(dropID: string, initialStock: number, initialSizes: SizeInfo[], live: boolean) {
  const [stock,     setStock]     = useState(initialStock);
  const [sizes,     setSizes]     = useState<SizeInfo[]>(initialSizes);
  const [connected, setConnected] = useState(false);
  useEffect(() => { setStock(initialStock); }, [initialStock]);
  useEffect(() => { setSizes(initialSizes); }, [initialSizes]);
  useEffect(() => {
    if (!live) return;
    const es = new EventSource(`/api/drops/${dropID}/events`);
    es.onopen    = () => setConnected(true);
    es.onerror   = () => setConnected(false);
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.stock   !== undefined) setStock(msg.stock);
        if (msg.sizes   !== undefined) setSizes(msg.sizes);
      } catch { /* */ }
    };
    return () => { es.close(); setConnected(false); };
  }, [dropID, live]);
  return { stock, sizes, connected };
}

function useReserve(dropID: string, itemID: string, size: string | null, token: string | null) {
  const [state, setState] = useState<ReserveState>({ status: "idle" });
  const reserve = useCallback(async () => {
    if (!token) { setState({ status: "error", message: "Not authenticated — try again.", retryable: true }); return; }
    if (!size)  { setState({ status: "error", message: "Select a size first.", retryable: false }); return; }
    setState({ status: "loading" });
    let lastErr = "";
    for (let att = 0; att < 3; att++) {
      try {
        const res = await fetch("/api/reserve", {
          method: "POST",
          headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
          body: JSON.stringify({ drop_id: dropID, item_id: itemID, size }),
          signal: AbortSignal.timeout(5_000),
        });
        const body = await res.json();
        if (res.status === 201) {
          // Save reservation so user can resume checkout if they leave
          try {
            localStorage.setItem(`dmsdy_reservation_${dropID}`, JSON.stringify({
              id: body.reservation_id,
              expires_at: body.expires_at,
            }));
          } catch {}
          setState({ status: "success", reservationID: body.reservation_id, expiresAt: body.expires_at });
          return;
        }
        if (res.status === 409 && body.reservation_id) {
          // Already have an active reservation — treat it as success and redirect
          try {
            localStorage.setItem(`dmsdy_reservation_${dropID}`, JSON.stringify({
              id: body.reservation_id,
              expires_at: body.expires_at,
            }));
          } catch {}
          setState({ status: "success", reservationID: body.reservation_id, expiresAt: body.expires_at });
          return;
        }
        if ([409, 410, 429, 401].includes(res.status)) {
          setState({ status: "error", message: body.error, retryable: false });
          return;
        }
        lastErr = body.error ?? "Server error";
      } catch (e: unknown) {
        lastErr = e instanceof Error && e.name === "TimeoutError" ? "Request timed out" : "Network error";
      }
      await new Promise((r) => setTimeout(r, 300 * 3 ** att + Math.random() * 150));
    }
    setState({ status: "error", message: lastErr, retryable: true });
  }, [dropID, itemID, size, token]);
  return { state, reserve, reset: () => setState({ status: "idle" }) };
}

function useWaitlist(dropID: string, token: string | null) {
  const [state, setState] = useState<WaitlistState>({ status: "idle" });
  const join = useCallback(async () => {
    if (!token) return;
    setState({ status: "loading" });
    try {
      const res = await fetch("/api/waitlist", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ drop_id: dropID }),
      });
      const body = await res.json();
      if (res.ok) { setState({ status: "joined", position: body.position }); return; }
      setState({ status: "error" });
    } catch { setState({ status: "error" }); }
  }, [dropID, token]);
  return { state, join };
}

// ─────────────────────────────────────────────────────────────────────────────
// DIGIT
// ─────────────────────────────────────────────────────────────────────────────
function Digit({ value, label, urgent }: { value: number; label: string; urgent?: boolean }) {
  const display = String(value).padStart(2, "0");
  return (
    <div className="flex flex-col items-center gap-2">
      <motion.div animate={urgent ? { borderColor: ["#52525b","#ef4444","#52525b"] } : {}}
        transition={{ duration: 0.5, repeat: urgent ? Infinity : 0 }}
        className="relative overflow-hidden border border-zinc-600 bg-zinc-950" style={{ width: 72, height: 80 }}>
        <div className="absolute inset-x-0 top-1/2 h-px bg-zinc-700 z-10" />
        <AnimatePresence mode="popLayout">
          <motion.span key={display}
            initial={{ y: "-110%", opacity: 0 }} animate={{ y: "0%", opacity: 1 }} exit={{ y: "110%", opacity: 0 }}
            transition={{ duration: 0.22, ease: [0.16, 1, 0.3, 1] }}
            className={`absolute inset-0 flex items-center justify-center text-4xl font-black tabular-nums ${urgent ? "text-red-400" : "text-white"}`}
            style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
            {display}
          </motion.span>
        </AnimatePresence>
        <div className="absolute inset-0 pointer-events-none opacity-20"
          style={{ background: "repeating-linear-gradient(0deg,transparent,transparent 3px,rgba(0,0,0,0.7) 4px)" }} />
      </motion.div>
      <span className={`text-xs tracking-widest uppercase font-mono ${urgent ? "text-red-500" : "text-zinc-400"}`}>{label}</span>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// STOCK BAR
// ─────────────────────────────────────────────────────────────────────────────
function StockBar({ stock, total }: { stock: number; total: number }) {
  const pct       = Math.max(0, Math.min(1, stock / total));
  const motionPct = useMotionValue(1);
  const barColor  = useTransform(motionPct, [0, 0.1, 0.4, 1], ["#ef4444","#f97316","#eab308","#ffffff"]);
  const width     = useTransform(motionPct, (v) => `${v * 100}%`);
  useEffect(() => { animate(motionPct, pct, { duration: 1.2, ease: [0.16, 1, 0.3, 1] }); }, [pct, motionPct]);
  const isCritical = pct > 0 && pct < 0.1;
  return (
    <div className="w-full space-y-3">
      <div className="flex justify-between items-center">
        <span className="text-xs tracking-widest uppercase font-mono text-zinc-400">Stock Remaining</span>
        <motion.span key={stock} initial={{ opacity: 0, y: -5 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.3 }}
          className={`text-sm font-mono font-bold tabular-nums ${isCritical ? "text-red-400" : "text-white"}`}>
          {stock} <span className="text-zinc-500 font-normal">/ {total}</span>
        </motion.span>
      </div>
      <div className="relative h-1 w-full bg-zinc-800 overflow-hidden">
        <motion.div className="absolute left-0 top-0 h-full" style={{ width, backgroundColor: barColor }} />
      </div>
      <AnimatePresence>
        {isCritical && (
          <motion.div initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }} className="flex items-center gap-2">
            <motion.div animate={{ opacity: [1, 0.2, 1] }} transition={{ duration: 0.8, repeat: Infinity }}
              className="w-1.5 h-1.5 rounded-full bg-red-500" />
            <span className="text-xs font-mono tracking-widest uppercase text-red-400">Critical — {stock} units left</span>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// PURCHASE BUTTON
// ─────────────────────────────────────────────────────────────────────────────
function PurchaseButton({ phase, reserveState, onPress, price, sizeSelected }: {
  phase: DropPhase; reserveState: ReserveState; onPress: () => void;
  price: string; sizeSelected: boolean;
}) {
  const isLoading = reserveState.status === "loading";
  const isSuccess = reserveState.status === "success";
  const isSoldOut = phase === "sold_out" || phase === "ended";
  const isPre     = phase === "pre";
  const noSize    = !sizeSelected && !isSoldOut && !isPre;
  const disabled  = isPre || isSoldOut || isLoading || isSuccess || noSize;

  const label = isPre     ? "DROP NOT STARTED"
    : isSoldOut           ? "SOLD OUT"
    : isLoading           ? "SECURING…"
    : isSuccess           ? "✓ SECURED"
    : noSize              ? "SELECT A SIZE"
    : `SECURE YOURS — ${price}`;

  const { display, scramble } = useScramble(label);

  return (
    <motion.button
      onClick={onPress}
      disabled={disabled}
      onMouseEnter={() => { if (!disabled) scramble(); }}
      whileTap={disabled ? {} : { scale: 0.985 }}
      className={[
        "group relative w-full h-16 overflow-hidden text-sm tracking-widest uppercase font-mono font-bold border transition-colors duration-150",
        isSuccess  ? "bg-white text-black border-white cursor-default"
        : isSoldOut ? "bg-transparent text-zinc-600 border-zinc-700 cursor-not-allowed"
        : isPre     ? "bg-transparent text-zinc-500 border-zinc-700 cursor-not-allowed"
        : noSize    ? "bg-transparent text-zinc-600 border-zinc-800 cursor-not-allowed"
        : isLoading ? "bg-transparent text-zinc-300 border-zinc-500 cursor-wait"
        :             "bg-transparent text-white border-white cursor-pointer",
      ].join(" ")}
    >
      {!disabled && !isLoading && (
        <motion.span aria-hidden className="absolute inset-0 bg-white origin-left z-0"
          initial={{ scaleX: 0 }} whileHover={{ scaleX: 1 }}
          transition={{ duration: 0.22, ease: [0.16, 1, 0.3, 1] }} />
      )}
      <span className="relative z-10 flex items-center justify-center gap-3 group-hover:text-black transition-colors duration-150">
        {isLoading && (
          <motion.span animate={{ rotate: 360 }} transition={{ duration: 0.9, repeat: Infinity, ease: "linear" }}
            className="w-4 h-4 border-2 border-zinc-400 border-t-transparent rounded-full inline-block" />
        )}
        {display}
      </span>
    </motion.button>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// REMAINING SMALL COMPONENTS
// ─────────────────────────────────────────────────────────────────────────────
function ReservationOverlay({ expiresAt, reservationID, dropID }: { expiresAt: string; reservationID: string; dropID: string }) {
  const router = useRouter();
  const time   = useCountdown(new Date(expiresAt));
  const urgent = time.minutes < 2;
  return (
    <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4 }}
      className="border border-zinc-500 bg-zinc-950 p-5 space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs tracking-widest uppercase font-mono text-zinc-400 mb-1">Item Secured</p>
          <p className="text-sm font-mono text-zinc-300">Complete checkout before time expires.</p>
        </div>
        <div className="text-right shrink-0">
          <p className="text-xs tracking-widest uppercase font-mono text-zinc-500 mb-1">Expires</p>
          <motion.p animate={urgent ? { color: ["#ef4444","#ffffff","#ef4444"] } : {}}
            transition={{ duration: 0.8, repeat: urgent ? Infinity : 0 }}
            className="text-3xl font-black tabular-nums text-white"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
            {String(time.minutes).padStart(2,"0")}:{String(time.seconds).padStart(2,"0")}
          </motion.p>
        </div>
      </div>
      <button onClick={() => router.push(`/checkout/${reservationID}?drop=${dropID}&expires=${encodeURIComponent(expiresAt)}`)}
        className="w-full h-12 text-sm tracking-widest uppercase font-mono font-bold bg-white text-black border border-white hover:bg-zinc-200 transition-colors">
        Proceed to Checkout →
      </button>
    </motion.div>
  );
}

function WaitlistPanel({ state, join }: { state: WaitlistState; join: () => void }) {
  if (state.status === "joined") return (
    <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}
      className="border border-zinc-700 bg-zinc-950 p-4 space-y-1">
      <p className="text-xs tracking-widest uppercase font-mono text-zinc-400">Waitlist Position</p>
      <p className="text-2xl font-black text-white" style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>#{state.position}</p>
      <p className="text-xs font-mono text-zinc-500">You will be notified if stock becomes available.</p>
    </motion.div>
  );
  return (
    <motion.button onClick={join} disabled={state.status === "loading"}
      className="w-full h-12 text-xs tracking-widest uppercase font-mono font-bold border border-zinc-700 text-zinc-400 hover:border-zinc-500 hover:text-zinc-300 transition-colors disabled:opacity-50">
      {state.status === "loading" ? "Joining…" : "Join Waitlist"}
    </motion.button>
  );
}

function Ticker({ items }: { items: string[] }) {
  const text = items.join("  ·  ") + "  ·  ";
  return (
    <div className="overflow-hidden border-y border-zinc-800 py-2.5 bg-black">
      <motion.div animate={{ x: ["0%","-50%"] }} transition={{ duration: 28, repeat: Infinity, ease: "linear" }}
        className="flex whitespace-nowrap">
        {[text, text].map((t, i) => (
          <span key={i} className="text-xs tracking-widest uppercase font-mono text-zinc-500 pr-8">{t}</span>
        ))}
      </motion.div>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOT
// ─────────────────────────────────────────────────────────────────────────────
const ITEM_ID = "jacket-001";
const SPECS   = ["Waxed Ventile® Shell","YKK® Aquaguard Zip","D-ring Utility"];

// Photos per drop — put files in /public/product/{dropID}/1.jpg, 2.jpg …
// Set how many photos each drop has. Unknown drops default to 4.
const PHOTOS_BY_DROP: Record<string, number> = {
  "dmsdy-ss25-001": 4,
  "dmsdy-ss25-002": 4,
  "dmsdy-ss25-003": 4,
  "dmsdy-ss25-004": 4,
  "dmsdy-ss25-005": 4,
  "dmsdy-fw25-001": 4,
};
const DEFAULT_PHOTO_COUNT = 4;

function getPhotos(dropID: string): string[] {
  const count = PHOTOS_BY_DROP[dropID] ?? DEFAULT_PHOTO_COUNT;
  return Array.from({ length: count }, (_, i) => `/product/${dropID}/${i + 1}.jpg`);
}

export default function DoomsdayDrop({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const DROP_ID = id;
  const router  = useRouter();
  const PHOTOS  = getPhotos(DROP_ID);
  const dropNum = DROP_ID.split("-").pop()?.toUpperCase() ?? "001";
  const EDITION = `SS/25 — COLLECTION ${dropNum}`;

  const [booted,       setBooted]       = useState(false);
  const [soldOutFlash, setSoldOutFlash] = useState(false);
  const [selectedSize, setSelectedSize] = useState<string | null>(null);
  const [authOpen,     setAuthOpen]     = useState(false);
  const prevStockRef = useRef<number | null>(null);
  const prevSecRef   = useRef(0);

  const { token, isUser, email, handleVerified, logout } = useAuth();
  const { data, loading, error } = useDropData(DROP_ID);

  const startsAt = data ? new Date(data.starts_at) : null;
  const endsAt   = data ? new Date(data.ends_at)   : null;

  const [phase, setPhase] = useState<DropPhase>("pre");
  useEffect(() => {
    if (!startsAt || !endsAt) return;
    const tick = () => {
      const now = Date.now();
      if      (now < startsAt.getTime()) setPhase("pre");
      else if (now > endsAt.getTime())   setPhase("ended");
      else                               setPhase("live");
    };
    tick();
    const id = setInterval(tick, 1_000);
    return () => clearInterval(id);
  }, [data]);

  const countdownTarget = phase === "pre" ? startsAt : endsAt;
  const timeLeft        = useCountdown(countdownTarget);

  const isUrgent = phase === "pre"
    && timeLeft.days === 0 && timeLeft.hours === 0
    && timeLeft.minutes === 0 && timeLeft.seconds <= 10 && timeLeft.seconds > 0;

  const [shaking, setShaking] = useState(false);
  useEffect(() => {
    if (isUrgent && timeLeft.seconds !== prevSecRef.current) {
      prevSecRef.current = timeLeft.seconds;
      setShaking(true);
      setTimeout(() => setShaking(false), 320);
    }
  }, [timeLeft.seconds, isUrgent]);

  const { stock, sizes, connected } = useSSEStock(
    DROP_ID, data?.stock_remaining ?? 0, data?.sizes ?? [], phase === "live"
  );

  const effectivePhase: DropPhase = phase === "live" && stock === 0 ? "sold_out" : phase;
  const isCritical = stock > 0 && stock / (data?.total_stock ?? 100) < 0.1;

  // If selected size runs out, deselect
  useEffect(() => {
    if (selectedSize) {
      const s = sizes.find((sz) => sz.label === selectedSize);
      if (s && s.stock === 0) setSelectedSize(null);
    }
  }, [sizes, selectedSize]);

  useEffect(() => {
    if (prevStockRef.current !== null && prevStockRef.current > 0 && stock === 0) {
      setSoldOutFlash(true);
      setTimeout(() => setSoldOutFlash(false), 1_500);
    }
    prevStockRef.current = stock;
  }, [stock]);

  const { state: reserveState, reserve, reset } = useReserve(DROP_ID, ITEM_ID, selectedSize, token);
  const { state: waitlistState, join }           = useWaitlist(DROP_ID, token);

  // Gate purchase behind auth — open modal if not verified user
  // If user has an active reservation already, redirect straight to checkout
  const handleBuy = useCallback(() => {
    if (!isUser) { setAuthOpen(true); return; }
    // Check for existing active reservation for this drop
    const saved = localStorage.getItem(`dmsdy_reservation_${DROP_ID}`);
    if (saved) {
      try {
        const r = JSON.parse(saved) as { id: string; expires_at: string };
        if (new Date(r.expires_at) > new Date()) {
          router.push(`/checkout/${r.id}?drop=${DROP_ID}&expires=${r.expires_at}`);
          return;
        } else {
          localStorage.removeItem(`dmsdy_reservation_${DROP_ID}`);
        }
      } catch {}
    }
    reserve();
  }, [isUser, reserve, DROP_ID, router]);

  const titleText = data?.name ?? "WRAITH FIELD JACKET";
  const { display: titleDisplay, scramble: titleScramble } = useScramble(titleText);
  const hasScrambledOnce = useRef(false);
  useEffect(() => {
    if (data?.name && !hasScrambledOnce.current) {
      hasScrambledOnce.current = true;
      setTimeout(titleScramble, 300);
    }
  }, [data?.name, titleScramble]);

  const price     = data ? `$${Math.round(data.price_cents / 100)}` : "—";
  const isSuccess = reserveState.status === "success";
  const isSoldOut = effectivePhase === "sold_out" || effectivePhase === "ended";

  if (!booted) return (
    <>
      <style>{GLOBAL_CSS}</style>
      <CustomCursor />
      <BootSequence onDone={() => setBooted(true)} />
    </>
  );

  if (loading) return (
    <div className="min-h-screen w-full bg-black flex items-center justify-center"
      style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>
      <motion.div animate={{ opacity: [0.3, 1, 0.3] }} transition={{ duration: 1.5, repeat: Infinity }}
        className="text-xs font-mono tracking-widest uppercase text-zinc-600">Loading drop…</motion.div>
    </div>
  );

  if (error) return (
    <div className="min-h-screen bg-black flex items-center justify-center">
      <p className="text-sm font-mono text-red-400 tracking-widest uppercase">Failed to load drop data.</p>
    </div>
  );

  return (
    <div className="h-screen w-full bg-black text-white flex flex-col overflow-hidden"
      style={{ fontFamily: "'IBM Plex Mono','Courier New',monospace" }}>

      <style>{GLOBAL_CSS}</style>
      <CustomCursor />
      <SoldOutFlash show={soldOutFlash} />
      <div className="crt-overlay" aria-hidden />

      {/* Auth modal */}
      <AuthModal
        open={authOpen}
        onClose={() => setAuthOpen(false)}
        onSuccess={(t, e) => { handleVerified(t, e); setAuthOpen(false); }}
      />

      <div aria-hidden className="fixed inset-0 pointer-events-none z-0 opacity-[0.035]"
        style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`, backgroundSize: "160px 160px" }} />

      <AnimatePresence>
        {isCritical && (
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: [0, 0.6, 0] }}
            transition={{ duration: 2.5, repeat: Infinity }}
            className="fixed inset-0 pointer-events-none z-[2]"
            style={{ boxShadow: "inset 0 0 80px rgba(239,68,68,0.2)" }} />
        )}
      </AnimatePresence>

      {/* HEADER */}
      <header className="relative z-10 flex-shrink-0 flex items-center justify-between px-6 py-4 border-b border-zinc-800">
        <div className="flex items-center gap-4">
          <ScrambleText text="DOOMSDAY™" tag="span"
            className="text-2xl font-black text-white"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "0.04em" }} />
          <span className="hidden sm:block text-zinc-700">|</span>
          <button onClick={() => router.push("/drops")}
            className="hidden sm:block text-xs font-mono tracking-widest uppercase text-zinc-500 hover:text-zinc-300 transition-colors">
            All Drops
          </button>
          <span className="hidden sm:block text-zinc-800">/</span>
          <span className="hidden sm:block text-xs font-mono tracking-widest uppercase text-zinc-500">DROP #{dropNum}</span>
        </div>
        <div className="flex items-center gap-4">
          {phase === "live" && (
            <div className="hidden sm:flex items-center gap-1.5">
              <motion.div animate={{ opacity: connected ? [1, 0.3, 1] : 1 }}
                transition={{ duration: 1, repeat: connected ? Infinity : 0 }}
                className={`w-1.5 h-1.5 rounded-full ${connected ? "bg-green-500" : "bg-zinc-600"}`} />
              <span className="text-xs font-mono text-zinc-600">{connected ? "live" : "reconnecting"}</span>
            </div>
          )}
          {/* Auth status */}
          {isUser ? (
            <div className="hidden sm:flex items-center gap-3">
              <span className="text-xs font-mono text-zinc-500 truncate max-w-[140px]">{email}</span>
              <button onClick={logout}
                className="text-xs font-mono tracking-widest uppercase text-zinc-600 hover:text-zinc-400 transition-colors">
                Log out
              </button>
            </div>
          ) : (
            <button onClick={() => setAuthOpen(true)}
              className="hidden sm:block text-xs font-mono tracking-widest uppercase border border-zinc-700 text-zinc-400 hover:border-zinc-500 hover:text-white transition-colors px-3 py-1.5">
              Sign In
            </button>
          )}
          <div className="flex items-center gap-2 border border-zinc-700 px-3 py-1.5">
            <motion.div animate={{ opacity: effectivePhase === "live" ? [1, 0.1, 1] : 1 }}
              transition={{ duration: 1, repeat: effectivePhase === "live" ? Infinity : 0 }}
              className={`w-1.5 h-1.5 rounded-full ${
                effectivePhase === "live" ? "bg-red-500"
                : effectivePhase === "pre" ? "bg-yellow-400" : "bg-zinc-600"}`} />
            <span className="text-xs font-mono tracking-widest uppercase text-zinc-300">
              {effectivePhase === "pre" ? "Pre-Drop" : effectivePhase === "live" ? "Live"
               : effectivePhase === "sold_out" ? "Sold Out" : "Closed"}
            </span>
          </div>
        </div>
      </header>

      <div className="flex-shrink-0">
        <Ticker items={["WRAITH FIELD JACKET","SS/25 COLLECTION 001",
          `${data?.total_stock ?? 100} UNITS WORLDWIDE`,"NO RESTOCKS EVER",price,"DOOMSDAY™"]} />
      </div>

      {/* BODY */}
      <main className="relative z-10 flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-[1fr_480px]">

        {/* Left — photo viewer */}
        <div className="hidden lg:block border-r border-zinc-800 h-full">
          <PhotoViewer photos={PHOTOS} phase={effectivePhase} price={price} edition={EDITION} />
        </div>

        {/* Right — details */}
        <motion.div
          animate={shaking ? { x: [0,-7,7,-4,4,-2,0] } : { x: 0 }}
          transition={{ duration: 0.28 }}
          className={`h-full overflow-y-auto flex flex-col p-7 md:p-9 gap-6 ${isCritical ? "ring-1 ring-inset ring-red-900/30" : ""}`}
        >
          {/* Mobile photo */}
          <div className="lg:hidden h-72 -mx-7 md:-mx-9">
            <PhotoViewer photos={PHOTOS} phase={effectivePhase} price={price} edition={EDITION} />
          </div>

          {/* Title */}
          <div>
            <motion.p initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.2 }}
              className="text-xs font-mono tracking-widest uppercase text-zinc-500 mb-3">{EDITION}</motion.p>
            <motion.h1
              initial={{ opacity: 0, x: -16 }} animate={{ opacity: 1, x: 0 }}
              transition={{ delay: 0.3, duration: 0.55, ease: [0.16, 1, 0.3, 1] }}
              onMouseEnter={titleScramble}
              className="text-5xl md:text-6xl font-black leading-none uppercase text-white"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.01em", fontVariantNumeric: "tabular-nums" }}>
              {titleDisplay}
            </motion.h1>
            <motion.p initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.5 }}
              className="mt-4 text-sm text-zinc-400 leading-relaxed max-w-sm">{data?.description}</motion.p>
            <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.55 }}
              className="mt-4 flex flex-wrap gap-2">
              {SPECS.map((s) => (
                <span key={s} className="text-xs font-mono tracking-wide text-zinc-400 border border-zinc-700 px-2.5 py-1">{s}</span>
              ))}
            </motion.div>
          </div>

          {/* Countdown */}
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.4 }} className="space-y-3">
            <p className={`text-xs font-mono tracking-widest uppercase ${isUrgent ? "text-red-400" : "text-zinc-400"}`}>
              {effectivePhase === "pre" ? (isUrgent ? "⚠ IMMINENT" : "Drop Begins In")
               : effectivePhase === "live" ? "Drop Closes In" : "Drop Closed"}
            </p>
            {effectivePhase !== "ended" && effectivePhase !== "sold_out" ? (
              <div className="flex items-end gap-3">
                {timeLeft.days > 0 && <>
                  <Digit value={timeLeft.days}  label="Days" urgent={isUrgent} />
                  <span className={`text-3xl font-mono mb-7 leading-none ${isUrgent ? "text-red-500" : "text-zinc-500"}`}>:</span>
                </>}
                <Digit value={timeLeft.hours}   label="Hrs"  urgent={isUrgent} />
                <span className={`text-3xl font-mono mb-7 leading-none ${isUrgent ? "text-red-500" : "text-zinc-500"}`}>:</span>
                <Digit value={timeLeft.minutes} label="Min"  urgent={isUrgent} />
                <span className={`text-3xl font-mono mb-7 leading-none ${isUrgent ? "text-red-500" : "text-zinc-500"}`}>:</span>
                <Digit value={timeLeft.seconds} label="Sec"  urgent={isUrgent} />
              </div>
            ) : (
              <p className="text-3xl font-black uppercase text-zinc-500"
                style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
                {effectivePhase === "sold_out" ? "SOLD OUT" : "DROP ENDED"}
              </p>
            )}
          </motion.div>

          {/* Stock bar */}
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.55 }}>
            <StockBar stock={stock} total={data?.total_stock ?? 100} />
          </motion.div>

          <div className="h-px bg-zinc-800" />

          {/* Size selector */}
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.6 }}>
            {sizes.length > 0 && !isSoldOut && (
              <SizeSelector sizes={sizes} selected={selectedSize} onSelect={setSelectedSize} />
            )}
          </motion.div>

          {/* CTA */}
          <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.65 }} className="space-y-4">
            {isSoldOut && !isSuccess ? (
              <WaitlistPanel state={waitlistState} join={join} />
            ) : (
              <PurchaseButton
                phase={effectivePhase}
                reserveState={reserveState}
                onPress={handleBuy}
                price={price}
                sizeSelected={!!selectedSize}
              />
            )}

            <AnimatePresence>
              {reserveState.status === "error" && (
                <motion.div initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }} className="overflow-hidden">
                  <div className="flex items-start gap-3 p-4 border border-red-800 bg-red-950/20">
                    <span className="text-red-400 text-sm font-mono mt-0.5">✕</span>
                    <div className="flex-1 space-y-1.5">
                      <p className="text-sm font-mono text-red-300">{reserveState.message}</p>
                      {reserveState.retryable && (
                        <button onClick={() => { reset(); reserve(); }}
                          className="text-xs font-mono tracking-widest uppercase text-zinc-400 underline hover:text-white transition-colors">
                          Try Again
                        </button>
                      )}
                    </div>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>

            <AnimatePresence>
              {isSuccess && (
                <ReservationOverlay
                  expiresAt={(reserveState as Extract<ReserveState, { status: "success" }>).expiresAt}
                  reservationID={(reserveState as Extract<ReserveState, { status: "success" }>).reservationID}
                  dropID={DROP_ID}
                />
              )}
            </AnimatePresence>

            <p className="text-xs font-mono text-zinc-600 text-center tracking-wide">
              1 unit per customer · All sales final · No restocks · Ever.
            </p>
          </motion.div>

          <div className="mt-auto pt-4 border-t border-zinc-800 flex items-center justify-between">
            <span className="text-xs font-mono tracking-widest uppercase text-zinc-700">© DOOMSDAY MMXXV</span>
            <span className="text-xs font-mono tracking-widest uppercase text-zinc-700">100 Units · WW</span>
          </div>
        </motion.div>
      </main>
    </div>
  );
}