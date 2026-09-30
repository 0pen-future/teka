import { HvButton } from "@/components/hv";

interface LoadMoreButtonProps {
  loaded: number;
  total: number;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  onLoadMore: () => void;
}

/** "Xem thêm" footer for a page-by-page list; renders nothing once every row is loaded. */
export function LoadMoreButton({
  loaded,
  total,
  hasNextPage,
  isFetchingNextPage,
  onLoadMore,
}: LoadMoreButtonProps) {
  if (!hasNextPage) {
    return null;
  }
  return (
    <div className="flex flex-col items-center gap-1">
      <HvButton variant="secondary" size="sm" disabled={isFetchingNextPage} onClick={onLoadMore}>
        {isFetchingNextPage ? "Đang tải…" : "Xem thêm"}
      </HvButton>
      <p className="text-[12.5px] text-ink-500">
        Đang hiện {loaded} / {total}
      </p>
    </div>
  );
}
