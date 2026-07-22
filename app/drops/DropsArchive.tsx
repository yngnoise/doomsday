import Image from "next/image";
import Link from "next/link";

import type { DropItem } from "@/lib/dropApi";
import { getProductPreview } from "@/lib/productImages";

const fmtPrice = (cents: number) => `$${Math.round(cents / 100)}`;

const fmtDate = (iso: string) =>
  new Date(iso)
    .toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" })
    .toUpperCase();

const PHASE_META: Record<string, { label: string; color: string; dot: string }> = {
  pre: { label: "UPCOMING", color: "text-yellow-400", dot: "bg-yellow-400" },
  live: { label: "LIVE NOW", color: "text-green-400", dot: "bg-green-400" },
  sold_out: { label: "SOLD OUT", color: "text-zinc-500", dot: "bg-zinc-600" },
  ended: { label: "ENDED", color: "text-zinc-600", dot: "bg-zinc-700" },
};

export default function DropsArchive({
  drops,
  totalDrops,
}: {
  drops: DropItem[] | null;
  totalDrops: number | null;
}) {
  const live = drops?.filter(drop => drop.phase === "live") ?? [];
  const upcoming = drops?.filter(drop => drop.phase === "pre") ?? [];
  const past = drops?.filter(drop => drop.phase === "ended" || drop.phase === "sold_out") ?? [];
  const ordered = [...live, ...upcoming, ...past];
  const previewDrop = ordered[0];
  const count = totalDrops ?? ordered.length;

  return (
    <div
      className="min-h-dvh lg:h-dvh w-full bg-black text-white flex flex-col overflow-x-hidden lg:overflow-hidden"
      style={{ fontFamily: "var(--font-geist-mono), 'Courier New', monospace" }}
    >
      <div className="pointer-events-none fixed inset-0 z-[9990] bg-[repeating-linear-gradient(0deg,transparent,transparent_2px,rgba(255,255,255,0.035)_3px)]" aria-hidden />

      <header className="relative z-10 flex-shrink-0 flex flex-wrap items-center justify-between gap-3 px-4 sm:px-6 md:px-8 py-4 border-b border-zinc-800">
        <div className="flex items-center gap-4">
          <Link
            href="/drops"
            className="text-xl font-black text-white hover:text-zinc-300 transition-colors"
            style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "0.04em" }}
          >
            DOOMSDAY™
          </Link>
          <span aria-hidden className="text-zinc-700">|</span>
          <span className="hidden sm:inline text-xs tracking-widest uppercase text-zinc-500">All Drops</span>
        </div>
        <div className="flex items-center gap-3">
          {live.length > 0 && (
            <div className="flex items-center gap-2 border border-green-900 bg-green-950/30 px-3 py-1.5">
              <div className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" />
              <span className="text-xs tracking-widest uppercase text-green-400">{live.length} Live</span>
            </div>
          )}
          <span className="hidden sm:inline text-xs text-zinc-600">{count} drops total</span>
        </div>
      </header>

      <main className="flex-1 lg:min-h-0 flex">
        <div className="relative z-10 flex flex-col w-full lg:w-[52%] xl:w-[48%] border-r border-zinc-800 lg:overflow-hidden">
          <div className="flex-shrink-0 px-4 sm:px-8 pt-7 sm:pt-8 pb-6 border-b border-zinc-800">
            <p className="text-xs tracking-widest uppercase text-zinc-600 mb-2">DOOMSDAY™ DROP</p>
            <h1
              className="text-5xl sm:text-6xl md:text-7xl font-black text-white leading-none uppercase"
              style={{ fontFamily: "'Impact','Arial Black',sans-serif", letterSpacing: "-0.02em" }}
            >
              ARCHIVE
            </h1>
            <p className="text-xs text-zinc-600 mt-3">{count} drops · select a drop to enter</p>
          </div>

          <div className="flex-1 lg:overflow-y-auto">
            {drops === null ? (
              <p role="status" className="px-4 sm:px-8 py-12 text-xs text-zinc-500">
                Archive temporarily unavailable.
              </p>
            ) : ordered.length === 0 ? (
              <p className="px-4 sm:px-8 py-12 text-xs text-zinc-600">No drops found.</p>
            ) : (
              <div>
                {ordered.map((drop, index) => {
                  const phase = PHASE_META[drop.phase];
                  const ended = drop.phase === "ended" || drop.phase === "sold_out";
                  return (
                    <Link
                      key={drop.id}
                      href={`/drops/${drop.id}`}
                      className="group block border-b border-zinc-800 text-left last:border-0"
                    >
                      <div className="flex items-center gap-3 sm:gap-6 px-4 sm:px-8 py-5 relative">
                        <span className="text-xs text-zinc-600 w-8 flex-shrink-0 tabular-nums">
                          {String(index + 1).padStart(3, "0")}
                        </span>
                        <span
                          className={`flex-1 text-xl md:text-2xl font-black uppercase leading-none ${ended ? "text-zinc-500" : "text-white"}`}
                          style={{ fontFamily: "'Impact','Arial Black',sans-serif" }}
                        >
                          {drop.name}
                        </span>
                        <div className="hidden md:flex flex-col items-end gap-0.5 flex-shrink-0">
                          <span className="text-xs text-zinc-500 tracking-widest">{fmtDate(drop.starts_at)}</span>
                          <span className="text-xs text-zinc-600">{drop.total_stock} units</span>
                        </div>
                        <span className={`hidden sm:block text-sm font-bold w-16 text-right ${ended ? "text-zinc-600" : "text-white"}`}>
                          {fmtPrice(drop.price_cents)}
                        </span>
                        <div className="flex items-center gap-2 flex-shrink-0 sm:w-28 justify-end">
                          {drop.phase === "live" && <div className={`w-1.5 h-1.5 rounded-full animate-pulse ${phase.dot}`} />}
                          <span className={`text-xs tracking-widest uppercase ${phase.color}`}>{phase.label}</span>
                        </div>
                        <span aria-hidden className="hidden sm:block opacity-0 -translate-x-1.5 group-hover:opacity-100 group-hover:translate-x-0 transition-[opacity,transform]">→</span>
                        <div aria-hidden className="absolute inset-x-0 bottom-0 h-px bg-white origin-left scale-x-0 group-hover:scale-x-100 transition-transform" />
                      </div>
                    </Link>
                  );
                })}
                {totalDrops !== null && totalDrops > ordered.length && (
                  <Link
                    href="/drops?all=1"
                    className="block border-b border-zinc-800 px-4 sm:px-8 py-5 text-xs uppercase tracking-widest text-zinc-500 hover:text-white"
                  >
                    Load {totalDrops - ordered.length} more drops
                  </Link>
                )}
              </div>
            )}
          </div>

          <div className="flex-shrink-0 px-4 sm:px-8 py-4 border-t border-zinc-800 flex flex-wrap gap-2 items-center justify-between">
            <span className="text-xs tracking-widest uppercase text-zinc-700">© DOOMSDAY MMXXV</span>
            <span className="text-xs tracking-widest uppercase text-zinc-700">No restocks · Ever.</span>
          </div>
        </div>

        <div className="hidden lg:block flex-1 relative overflow-hidden bg-zinc-950">
          {previewDrop && (
            <>
              <Image
                src={getProductPreview(previewDrop.id)}
                alt=""
                aria-hidden
                fill
                className="object-cover scale-110 blur-2xl brightness-20 saturate-50"
                sizes="52vw"
              />
              <Image
                src={getProductPreview(previewDrop.id)}
                alt={previewDrop.name}
                fill
                className="object-contain object-center"
                sizes="52vw"
                priority
              />
            </>
          )}
        </div>
      </main>
    </div>
  );
}
