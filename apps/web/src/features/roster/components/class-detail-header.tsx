import { ArrowLeftIcon, CopyIcon } from "lucide-react";
import { Link, useLocation } from "react-router";

import { HvBadge, HvButton, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { copyToClipboard } from "@/lib/utils";

import { phaseLabel, phaseVariant } from "../lib/class-labels";
import type { Class } from "../schemas/roster-schemas";

interface ClassDetailHeaderProps {
  klass: Class;
  canWrite: boolean;
  onEdit: () => void;
}

/**
 * Where the back link returns: the list the class was opened from, passed as
 * router state by that list, else the full catalog. Only known list paths are
 * honored so arbitrary state never becomes a link target.
 */
function backTarget(state: unknown): { to: string; label: string } {
  if (
    typeof state === "object" &&
    state !== null &&
    "from" in state &&
    state.from === "/classes/recruiting"
  ) {
    return { to: "/classes/recruiting", label: "Lớp cần tuyển sinh" };
  }
  return { to: "/classes", label: "Danh sách lớp học" };
}

/** Back link, name, code with a copy button, phase badge and edit action. */
export function ClassDetailHeader({ klass, canWrite, onEdit }: ClassDetailHeaderProps) {
  const { has } = useCenterContext();
  const back = backTarget(useLocation().state);
  // The catalog page is gated on courses.read; without it the chip informs
  // instead of linking somewhere the member would bounce off.
  const canOpenCourse = has("courses.read");
  const courseChipClassName =
    "inline-flex items-center gap-1 rounded-full bg-sky-50 px-2.5 py-0.5 font-display text-[12px] font-bold text-sky-700";
  async function handleCopy() {
    const copied = await copyToClipboard(klass.code);
    if (copied) {
      hvToast("Đã sao chép mã lớp", { variant: "success" });
    } else {
      hvToast("Không sao chép được mã lớp", { variant: "danger" });
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <Link
        to={back.to}
        className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
      >
        <ArrowLeftIcon aria-hidden="true" className="size-4" />
        {back.label}
      </Link>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="font-display text-[26px] font-extrabold text-ink-900">{klass.name}</h1>
            <HvBadge variant={phaseVariant[klass.phase]} dot>
              {phaseLabel[klass.phase]}
            </HvBadge>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-[14px] text-ink-500">
            <span>Mã lớp: {klass.code}</span>
            <button
              type="button"
              onClick={() => void handleCopy()}
              disabled={klass.code === ""}
              className="inline-flex items-center gap-1 rounded-[var(--radius-sm)] px-1.5 py-0.5 font-display text-[12px] font-bold text-mint-600 hover:bg-mint-50 disabled:opacity-50"
            >
              <CopyIcon aria-hidden="true" className="size-3.5" />
              Sao chép
            </button>
            {klass.course && canOpenCourse ? (
              <Link
                to={`/courses/${klass.course.id}`}
                className={`${courseChipClassName} hover:bg-sky-100`}
              >
                Khóa: {klass.course.code} · {klass.course.name}
              </Link>
            ) : klass.course ? (
              <span className={courseChipClassName}>
                Khóa: {klass.course.code} · {klass.course.name}
              </span>
            ) : null}
          </div>
        </div>
        {canWrite ? (
          <HvButton variant="secondary" onClick={onEdit}>
            Sửa lớp
          </HvButton>
        ) : null}
      </div>
    </div>
  );
}
