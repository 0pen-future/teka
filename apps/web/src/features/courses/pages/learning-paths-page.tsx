import { SearchIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { z } from "zod";

import {
  HvBadge,
  HvButton,
  HvCard,
  HvSegmented,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { Input } from "@/components/ui/input";
import { useCenterContext } from "@/features/teaching";

import { PathDialog } from "../components/path-dialog";
import { usePathsList } from "../hooks/use-paths";
import { pathStatusLabel, pathStatusVariant, pathSummary } from "../lib/path-labels";
import { pathStatusSchema } from "../schemas/paths-schemas";

type StatusFilter = "all" | z.infer<typeof pathStatusSchema>;

const statusFilterSchema = z.union([z.literal("all"), pathStatusSchema]).catch("all");

const statusOptions: HvSegmentedOption<StatusFilter>[] = [
  { value: "all", label: "Tất cả" },
  { value: "draft", label: pathStatusLabel.draft },
  { value: "active", label: pathStatusLabel.active },
  { value: "archived", label: pathStatusLabel.archived },
];

const STATUS_ID_BASE = "paths-status";

/**
 * `/paths` — the center's learning paths: which courses a student should
 * take, in which order. Each card opens the stage timeline.
 */
export function LearningPathsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const status: StatusFilter = statusFilterSchema.parse(searchParams.get("status") ?? undefined);
  const { has, isResolved } = useCenterContext();
  const canEdit = has("paths.edit");

  const [query, setQuery] = useState("");
  const [q, setQ] = useState("");
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setQ(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const list = usePathsList(
    {
      status: status === "all" ? undefined : status,
      q: q || undefined,
      per_page: 100,
      sort: "name",
    },
    isResolved,
  );
  const paths = list.data?.items ?? [];

  function selectStatus(next: StatusFilter) {
    const params = new URLSearchParams(searchParams);
    if (next === "all") {
      params.delete("status");
    } else {
      params.set("status", next);
    }
    setSearchParams(params, { replace: true });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Lộ trình học</h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Lộ trình xếp khóa học theo giai đoạn để tư vấn học sinh nên học gì tiếp theo. Một khóa
            có thể nằm trong nhiều lộ trình.
          </p>
        </div>
        {canEdit ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            Tạo lộ trình
          </HvButton>
        ) : null}
      </div>

      <HvSegmented
        variant="tabs"
        idBase={STATUS_ID_BASE}
        aria-label="Lọc theo trạng thái"
        options={statusOptions}
        value={status}
        onValueChange={selectStatus}
      />

      <div className="relative">
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
        />
        <Input
          type="search"
          aria-label="Tìm lộ trình"
          placeholder="Tìm theo tên hoặc mã…"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="pl-9"
        />
      </div>

      {!isResolved || list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải lộ trình học" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được lộ trình học"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : paths.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={
            q
              ? "Không có lộ trình nào khớp từ khoá."
              : status !== "all"
                ? "Không có lộ trình nào ở trạng thái này."
                : "Chưa có lộ trình nào."
          }
          description={
            q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : status !== "all"
                ? "Chọn Tất cả để xem toàn bộ lộ trình."
                : canEdit
                  ? "Tạo lộ trình đầu tiên bằng nút Tạo lộ trình."
                  : "Người có quyền quản lý lộ trình sẽ thêm lộ trình tại đây."
          }
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {paths.map((path) => (
            <HvCard
              key={path.id}
              variant="flat"
              padding="sm"
              className="flex flex-col gap-2"
              role="article"
              aria-label={path.name}
            >
              <div className="flex items-start justify-between gap-2">
                <span className="font-mono text-[13px] text-ink-700">{path.code}</span>
                <HvBadge variant={pathStatusVariant[path.status]} size="sm" dot>
                  {pathStatusLabel[path.status]}
                </HvBadge>
              </div>
              <Link
                to={`/paths/${path.id}`}
                className="font-display text-[17px] font-extrabold text-ink-900 hover:text-mint-600"
              >
                {path.name}
              </Link>
              <p className="text-[13px] font-bold text-ink-500">{pathSummary(path)}</p>
              {path.description ? (
                <p className="line-clamp-2 text-[13px] text-ink-700">{path.description}</p>
              ) : null}
            </HvCard>
          ))}
        </div>
      )}

      {canEdit ? (
        <PathDialog
          mode="create"
          open={creating}
          onOpenChange={setCreating}
          onCreated={(path) => {
            hvToast(`Đã tạo lộ trình ${path.code}`);
            void navigate(`/paths/${path.id}`);
          }}
        />
      ) : null}
    </div>
  );
}
