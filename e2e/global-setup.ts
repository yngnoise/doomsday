import { request, type FullConfig } from "@playwright/test";

const adminPassword = process.env.ADMIN_PASSWORD ?? "e2e-admin-password-with-32-characters";

export default async function globalSetup(_config: FullConfig) {
  const api = await request.newContext({ baseURL: "http://127.0.0.1:8080" });
  const login = await api.post("/api/admin/login", { data: { password: adminPassword } });
  if (!login.ok()) throw new Error(`Admin login failed: ${login.status()} ${await login.text()}`);
  const { token } = await login.json() as { token: string };
  const headers = { Authorization: `Bearer ${token}` };
  const suffix = Date.now().toString(36);

  const createDrop = async (name: string, stock: number) => {
    const startsAt = new Date(Date.now() - 60_000).toISOString();
    const endsAt = new Date(Date.now() + 60 * 60_000).toISOString();
    const response = await api.post("/api/admin/drops", {
      headers,
      data: {
        name,
        description: "Deterministic Playwright fixture",
        price_cents: 12300,
        total_stock: stock,
        sizes: ["S", "M", "L", "XL"],
        starts_at: startsAt,
        ends_at: endsAt,
      },
    });
    if (!response.ok()) throw new Error(`Drop creation failed: ${response.status()} ${await response.text()}`);
    return (await response.json() as { id: string }).id;
  };

  const liveName = `E2E LIVE ${suffix}`;
  const soldOutName = `E2E SOLD OUT ${suffix}`;
  const liveDropID = await createDrop(liveName, 12);
  const soldOutDropID = await createDrop(soldOutName, 1);
  const soldOut = await api.patch(`/api/admin/drops/${soldOutDropID}/stock`, {
    headers,
    data: { stock: 0 },
  });
  if (!soldOut.ok()) throw new Error(`Sold-out setup failed: ${soldOut.status()} ${await soldOut.text()}`);

  process.env.E2E_LIVE_DROP_ID = liveDropID;
  process.env.E2E_LIVE_DROP_NAME = liveName;
  process.env.E2E_SOLD_OUT_DROP_ID = soldOutDropID;
  await api.dispose();
}
