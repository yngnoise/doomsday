import { fetchDrops } from "@/lib/dropApi";

import DropsArchive from "./DropsArchive";

export const dynamic = "force-dynamic";

const INITIAL_DROP_LIMIT = 12;

export default async function DropsArchivePage({
  searchParams,
}: {
  searchParams: Promise<{ all?: string }>;
}) {
  const showAll = (await searchParams).all === "1";
  const initialDrops = await fetchDrops().catch((error: unknown) => {
    console.error("Unable to server-render drop archive data", error);
    return null;
  });

  return (
    <DropsArchive
      drops={initialDrops ? (showAll ? initialDrops : initialDrops.slice(0, INITIAL_DROP_LIMIT)) : null}
      totalDrops={initialDrops?.length ?? null}
    />
  );
}
