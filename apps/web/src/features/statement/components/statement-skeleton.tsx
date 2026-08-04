import { Skeleton } from "@/components/ui/skeleton";

/**
 * Mirrors the final layout's rhythm — header block, child card, dark total
 * block, QR square — so the page does not visibly jump once data arrives.
 */
export function StatementSkeleton() {
  return (
    <div aria-hidden="true" className="flex flex-col gap-4">
      <Skeleton className="h-28 rounded-[var(--radius-xl)] bg-cream-200" />
      <Skeleton className="h-44 rounded-[var(--radius-xl)] bg-cream-200" />
      <div className="flex flex-col gap-3 rounded-[var(--radius-xl)] bg-cream-200 p-5">
        <Skeleton className="h-4 w-1/2 rounded-full bg-cream-300" />
        <Skeleton className="h-8 w-2/3 rounded-full bg-cream-300" />
        <Skeleton className="mx-auto size-[150px] rounded-[var(--radius-lg)] bg-cream-300" />
      </div>
    </div>
  );
}
