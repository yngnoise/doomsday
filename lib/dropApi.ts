export type DropPhase = "pre" | "live" | "sold_out" | "ended";

export interface DropItem {
  id: string;
  name: string;
  price_cents: number;
  total_stock: number;
  starts_at: string;
  ends_at: string;
  stock_remaining: number;
  phase: DropPhase;
}

export interface SizeInfo {
  label: string;
  stock: number;
}

export interface DropData extends DropItem {
  description: string;
  sizes: SizeInfo[];
}

function apiOrigin() {
  const raw = (process.env.API_ORIGIN ?? process.env.NEXT_PUBLIC_API_URL)?.replace(/\/$/, "");
  if (!raw) throw new Error("API_ORIGIN is required for server-side drop data");
  return /^https?:\/\//.test(raw) ? raw : `http://${raw}`;
}

async function fetchAPI<T>(path: string): Promise<T> {
  const response = await fetch(`${apiOrigin()}${path}`, { cache: "no-store" });
  if (!response.ok) throw new Error(`Drop API request failed with ${response.status}`);
  return response.json() as Promise<T>;
}

export function fetchDrops() {
  return fetchAPI<DropItem[]>("/api/drops");
}

export function fetchDrop(dropID: string) {
  return fetchAPI<DropData>(`/api/drops/${encodeURIComponent(dropID)}`);
}
