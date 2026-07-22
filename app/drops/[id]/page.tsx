import { fetchDrop } from "@/lib/dropApi";

import DropPageClient from "./DropPageClient";

export const dynamic = "force-dynamic";

export default async function DropPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const initialData = await fetchDrop(id).catch((error: unknown) => {
    console.error("Unable to server-render drop data", error);
    return null;
  });

  return <DropPageClient dropID={id} initialData={initialData} />;
}
