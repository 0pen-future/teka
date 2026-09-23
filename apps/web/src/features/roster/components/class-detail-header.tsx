import { ArrowLeftIcon, CopyIcon } from "lucide-react";
import { Link } from "react-router";

import { HvBadge, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { copyToClipboard } from "@/lib/utils";

import { phaseLabel, phaseVariant } from "../lib/class-labels";
import type { Class } from "../schemas/roster-schemas";

interface ClassDetailHeaderProps {
  klass: Class;
  canWrite: boolean;
}

/** Back link, name, code with a copy button, phase badge and the settings shortcut. */
export function ClassDetailHeader({ klass, canWrite }: ClassDetailHeaderProps) {
  const { has } = useCenterContext();
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
        to="/classes"
        className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
      >
        <ArrowLeftIcon aria-hidden="true" className="size-4" />
        Danh sách lớp học
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
          <Link
            to={`/classes/${klass.id}/settings`}
            className="inline-flex min-h-[44px] items-center rounded-[var(--radius-md)] bg-white px-[18px] font-display text-[length:var(--text-sm)] font-bold text-mint-600 shadow-[0_var(--press-depth)_0_var(--line-300),inset_0_0_0_2px_var(--line-200)] hover:bg-cream-100"
          >
            Sửa lớp
          </Link>
        ) : null}
      </div>
    </div>
  );
}
