"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import {
  useMotionValue, animate,
} from "framer-motion";
import SafeProductImage from "@/components/SafeProductImage";
import { getProductPreview } from "@/lib/productImages";

// ─────────────────────────────────────────────────────────────────────────────
// TYPES
// ─────────────────────────────────────────────────────────────────────────────
interface DropItem {
  id: string;
  name: string;
  price_cents: number;
  total_stock: number;
  starts_at: string;
  ends_at: string;
  stock_remaining: number;
  phase: "pre" | "live" | "sold_out" | "ended";
}

// ─────────────────────────────────────────────────────────────────────────────
// GLOBAL STYLES
// ─────────────────────────────────────────────────────────────────────────────
const GLOBAL_CSS = `
  @media (hover: hover) and (pointer: fine) {
    * { cursor: none !important; }
  }
  .crt-overlay {
    pointer-events: none; position: fixed; inset: 0; z-index: 9990;
  }
  .crt-overlay::before {
    content: ''; position: absolute; inset: 0;
    background: repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(255,255,255,0.035) 3px);
  }
  .crt-overlay::after {
    content: ''; position: absolute; inset: 0;
    background: radial-gradient(ellipse at center, transparent 58%, rgba(0,0,0,0.6) 100%);
  }
`;

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────
const fmtPrice = (cents: number) => `$${Math.round(cents / 100)}`;

const fmtDate = (iso: string) =>
  new Date(iso).toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" }).toUpperCase();

const PHASE_META: Record<string, { label: string; color: string; dot: string }> = {
  pre:      { label: "UPCOMING",  color: "text-yellow-400", dot: "bg-yellow-400" },
  live:     { label: "LIVE NOW",  color: "text-green-400",  dot: "bg-green-400"  },
  sold_out: { label: "SOLD OUT",  color: "text-zinc-500",   dot: "bg-zinc-600"   },
  ended:    { label: "ENDED",     color: "text-zinc-600",   dot: "bg-zinc-700"   },
};

// ─────────────────────────────────────────────────────────────────────────────
// SCRAMBLE
// ─────────────────────────────────────────────────────────────────────────────
const SCRAMBLE_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ01#@";

function useScramble(original: string) {
  const [display, setDisplay] = useState(original);
  const frameRef   = useRef<ReturnType<typeof setInterval> | undefined>(undefined);
  const runningRef = useRef(false);

  const scramble = () => {
    if (runningRef.current) return;
    const chars = original.split("");
    const totalFrames = original.replace(/ /g, "").length * 3;
    const scrambleSet = new Set(
      chars.map((ch, i) => ch !== " " ? i : -1)
        .filter(i => i !== -1)
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
  };

  useEffect(() => () => clearInterval(frameRef.current), []);
  return { display, scramble };
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
// DROP ROW
// ─────────────────────────────────────────────────────────────────────────────
function DropRow({ drop, index, isHovered, onHover, onLeave, onClick }: {
  drop: DropItem;
  index: number;
  isHovered: boolean;
  onHover: () => void;
  onLeave: () => void;
  onClick: () => void;
}) {
  const phase = PHASE_META[drop.phase];
  const isEnded = drop.phase === "ended" || drop.phase === "sold_out";
  const { display, scramble } = useScramble(drop.name);

  return (
    <motion.button
      type="button"
      onMouseEnter={() => { onHover(); scramble(); }}
      onMouseLeave={onLeave}
      onFocus={() => { onHover(); scramble(); }}
      onBlur={onLeave}
      onClick={onClick}
      className="group block w-full border-b border-zinc-800 text-left last:border-0"
      style={{ cursor: "none" }}
    >
      <div className="flex items-center gap-3 sm:gap-6 px-4 sm:px-8 py-5 relative">

        {/* Index number */}
        <span className="text-xs font-mono text-zinc-600 w-8 flex-shrink-0 tabular-nums">
          {String(index + 1).padStart(3, "0")}
        </span>

        {/* Name */}
        <span
          className={`flex-1 text-xl md:text-2xl font-black uppercase leading-none tabular-nums ${
            isEnded ? "text-zinc-500" : "text-white"
          }`}
          style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.01em" }}
        >
          {isEnded ? drop.name : display}
        </span>

        {/* Meta — date + stock */}
        <div className="hidden md:flex flex-col items-end gap-0.5 flex-shrink-0">
          <span className="text-xs font-mono text-zinc-500 tracking-widest">
            {fmtDate(drop.starts_at)}
          </span>
          <span className="text-xs font-mono text-zinc-600">
            {drop.total_stock} units
          </span>
        </div>

        {/* Price */}
        <span className={`hidden sm:block text-sm font-mono font-bold flex-shrink-0 w-16 text-right ${
          isEnded ? "text-zinc-600" : "text-white"
        }`}>
          {fmtPrice(drop.price_cents)}
        </span>

        {/* Status */}
        <div className="flex items-center gap-2 flex-shrink-0 sm:w-28 justify-end">
          {drop.phase === "live" && (
            <motion.div animate={{ opacity: [1, 0.2, 1] }} transition={{ duration: 1, repeat: Infinity }}
              className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${phase.dot}`} />
          )}
          <span className={`text-xs font-mono tracking-widest uppercase ${phase.color}`}>
            {phase.label}
          </span>
        </div>

        {/* Arrow — only visible on hover */}
        <motion.span
          animate={{ opacity: isHovered ? 1 : 0, x: isHovered ? 0 : -6 }}
          transition={{ duration: 0.15 }}
          className="hidden sm:block text-white text-sm font-mono flex-shrink-0 ml-2"
        >
          →
        </motion.span>

        {/* Hover line */}
        <motion.div
          className="absolute inset-x-0 bottom-0 h-px bg-white origin-left"
          initial={{ scaleX: 0 }}
          animate={{ scaleX: isHovered ? 1 : 0 }}
          transition={{ duration: 0.2, ease: [0.16, 1, 0.3, 1] }}
        />
      </div>
    </motion.button>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOT
// ─────────────────────────────────────────────────────────────────────────────
export default function DropsArchive() {
  const router = useRouter();

  const [drops,   setDrops]   = useState<DropItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [hovered, setHovered] = useState<string | null>(null);

  // Which photo is showing — separate state so photo lags slightly
  // behind hovered for a snappier feel
  const [activePhoto, setActivePhoto] = useState<string | null>(null);
  const photoTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    fetch("/api/drops")
      .then(r => r.json())
      .then((data: DropItem[]) => {
        setDrops(data);
        // Default photo = first drop
        if (data.length > 0) setActivePhoto(data[0].id);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, []);

  const handleHover = (id: string) => {
    setHovered(id);
    clearTimeout(photoTimerRef.current);
    // Tiny delay before swapping photo — feels more intentional
    photoTimerRef.current = setTimeout(() => setActivePhoto(id), 60);
  };

  const handleLeave = () => {
    setHovered(null);
    clearTimeout(photoTimerRef.current);
  };

  // Group drops
  const live     = drops.filter(d => d.phase === "live");
  const upcoming = drops.filter(d => d.phase === "pre");
  const past     = drops.filter(d => d.phase === "ended" || d.phase === "sold_out");

  const ordered = [...live, ...upcoming, ...past];

  // Title scramble on mount
  const { display: titleDisplay, scramble: titleScramble } = useScramble("ARCHIVE");
  const didScramble = useRef(false);
  useEffect(() => {
    if (!loading && !didScramble.current) {
      didScramble.current = true;
      setTimeout(titleScramble, 200);
    }
  }, [loading, titleScramble]);

  const hoveredDrop = hovered ? drops.find(d => d.id === hovered) : null;

  return (
    <div className="min-h-dvh lg:h-dvh w-full bg-black text-white flex flex-col overflow-x-hidden lg:overflow-hidden"
      style={{ fontFamily: "var(--font-geist-mono), 'Courier New', monospace" }}>

      <style>{GLOBAL_CSS}</style>
      <CustomCursor />
      <div className="crt-overlay" aria-hidden />

      {/* Grain */}
      <div aria-hidden className="fixed inset-0 pointer-events-none z-0 opacity-[0.03]"
        style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`, backgroundSize: "160px 160px" }} />

      {/* HEADER */}
      <header className="relative z-10 flex-shrink-0 flex flex-wrap items-center justify-between gap-3 px-4 sm:px-6 md:px-8 py-4 border-b border-zinc-800">
        <div className="flex items-center gap-4">
          <button onClick={() => router.push("/drops")}
            className="text-xl font-black text-white hover:text-zinc-300 transition-colors"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "0.04em" }}>
            DOOMSDAY™
          </button>
          <span aria-hidden className="text-zinc-700">|</span>
          <span className="hidden sm:inline text-xs font-mono tracking-widest uppercase text-zinc-500">All Drops</span>
        </div>
        <div className="flex items-center gap-3">
          {live.length > 0 && (
            <div className="flex items-center gap-2 border border-green-900 bg-green-950/30 px-3 py-1.5">
              <motion.div animate={{ opacity: [1, 0.2, 1] }} transition={{ duration: 1, repeat: Infinity }}
                className="w-1.5 h-1.5 rounded-full bg-green-400" />
              <span className="text-xs font-mono tracking-widest uppercase text-green-400">
                {live.length} Live
              </span>
            </div>
          )}
          <span className="hidden sm:inline text-xs font-mono text-zinc-600">{drops.length} drops total</span>
        </div>
      </header>

      {/* BODY */}
      <main className="flex-1 lg:min-h-0 flex">

        {/* ── LEFT — index ─────────────────────────────────────────── */}
        <div className="relative z-10 flex flex-col w-full lg:w-[52%] xl:w-[48%] border-r border-zinc-800 lg:overflow-hidden">

          {/* Title block */}
          <div className="flex-shrink-0 px-4 sm:px-8 pt-7 sm:pt-8 pb-6 border-b border-zinc-800">
            <p className="text-xs font-mono tracking-widest uppercase text-zinc-600 mb-2">
              DOOMSDAY™ DROP
            </p>
            <h1
              onMouseEnter={titleScramble}
              className="text-5xl sm:text-6xl md:text-7xl font-black text-white leading-none uppercase"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.02em" }}>
              {titleDisplay}
            </h1>
            <p className="text-xs font-mono text-zinc-600 mt-3">
              {ordered.length} drops · hover to preview · click to enter
            </p>
          </div>

          {/* Drop list */}
          <div className="flex-1 lg:overflow-y-auto">
            {loading ? (
              <div aria-live="polite" aria-busy="true">
                <span className="sr-only">Loading drops</span>
                {Array.from({ length: 6 }, (_, index) => (
                  <div aria-hidden key={index} className="h-[81px] border-b border-zinc-800 px-4 sm:px-8 py-5">
                    <div className="h-6 w-3/4 bg-zinc-900 animate-pulse" />
                  </div>
                ))}
              </div>
            ) : ordered.length === 0 ? (
              <div className="px-8 py-12">
                <p className="text-xs font-mono text-zinc-600">No drops found.</p>
              </div>
            ) : (
              <div>
                {ordered.map((drop, i) => (
                  <div key={drop.id}>
                    <DropRow
                      drop={drop}
                      index={i}
                      isHovered={hovered === null ? true : hovered === drop.id}
                      onHover={() => handleHover(drop.id)}
                      onLeave={handleLeave}
                      onClick={() => router.push(`/drops/${drop.id}`)}
                    />
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="flex-shrink-0 px-4 sm:px-8 py-4 border-t border-zinc-800 flex flex-wrap gap-2 items-center justify-between">
            <span className="text-xs font-mono tracking-widest uppercase text-zinc-700">© DOOMSDAY MMXXV</span>
            <span className="text-xs font-mono tracking-widest uppercase text-zinc-700">No restocks · Ever.</span>
          </div>
        </div>

        {/* ── RIGHT — photo ─────────────────────────────────────────── */}
        <div className="hidden lg:block flex-1 relative overflow-hidden bg-zinc-950">

          {/* Photos — one per drop, instant swap */}
          {ordered.map(drop => (
            <div key={drop.id}
              className="absolute inset-0 transition-opacity duration-0"
              style={{ opacity: activePhoto === drop.id ? 1 : 0 }}>

              {/* Blurred bg */}
              <SafeProductImage
                src={getProductPreview(drop.id)}
                alt=""
                aria-hidden
                fill
                className="object-cover scale-110"
                style={{ filter: "blur(28px) brightness(0.2) saturate(0.5)" }}
                sizes="52vw"
              />

              {/* Main photo */}
              <SafeProductImage
                src={getProductPreview(drop.id)}
                alt={drop.name}
                fill
                className="object-contain object-center"
                sizes="52vw"
                priority={drop.id === ordered[0]?.id}
              />
            </div>
          ))}

          {/* Overlay info — appears when a row is hovered */}
          <AnimatePresence>
            {hoveredDrop && (
              <motion.div
                key={hoveredDrop.id}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: 8 }}
                transition={{ duration: 0.2 }}
                className="absolute bottom-0 left-0 right-0 z-10 p-8 bg-gradient-to-t from-black/90 via-black/40 to-transparent"
              >
                <p className="text-xs font-mono tracking-widest uppercase text-zinc-400 mb-1">
                  {fmtDate(hoveredDrop.starts_at)} · {fmtPrice(hoveredDrop.price_cents)} · {hoveredDrop.total_stock} units
                </p>
                <p className="text-4xl font-black text-white uppercase leading-none"
                  style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}>
                  {hoveredDrop.name}
                </p>
                <div className="flex items-center gap-2 mt-3">
                  <span className={`text-xs font-mono tracking-widest uppercase ${PHASE_META[hoveredDrop.phase].color}`}>
                    {PHASE_META[hoveredDrop.phase].label}
                  </span>
                  <span className="text-zinc-600 text-xs font-mono">·</span>
                  <span className="text-xs font-mono text-zinc-500">
                    Click to {hoveredDrop.phase === "ended" || hoveredDrop.phase === "sold_out" ? "view" : "enter drop"}
                  </span>
                </div>
              </motion.div>
            )}
          </AnimatePresence>

          {/* Default state — no hover */}
          <AnimatePresence>
            {!hoveredDrop && !loading && (
              <motion.div
                initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
                className="absolute bottom-8 left-8 z-10">
                <p className="text-xs font-mono tracking-widest uppercase text-zinc-600">
                  Hover a drop to preview
                </p>
              </motion.div>
            )}
          </AnimatePresence>

          {/* Corner marks */}
          {(["top-0 left-0 border-t border-l","top-0 right-0 border-t border-r",
             "bottom-0 left-0 border-b border-l","bottom-0 right-0 border-b border-r"] as const).map((cls) => (
            <div key={cls} className={`absolute w-6 h-6 ${cls} border-white/15 z-[5]`} />
          ))}
        </div>
      </main>
    </div>
  );
}
