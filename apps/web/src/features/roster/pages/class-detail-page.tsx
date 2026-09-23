import { Link, useParams, useSearchParams } from "react-router";
import { z } from "zod";

import { HvSegmented, HvStateBlock, type HvSegmentedOption } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";

import { ClassDetailHeader } from "../components/class-detail-header";
import { ClassInfoTab } from "../components/class-info-tab";
import { ClassSessionsTab } from "../components/class-sessions-tab";
import { ClassStudentsTab } from "../components/class-students-tab";
import { useClass } from "../hooks/use-classes";
import { canWriteClass } from "../lib/class-permissions";

const liveTabs = ["info", "students", "sessions"] as const;
type LiveTab = (typeof liveTabs)[number];

const tabSchema = z.enum(liveTabs).catch("info");

const LATER_PHASE_HINT = "Có ở phase sau";

const tabOptions: HvSegmentedOption<string>[] = [
  { value: "info", label: "Thông tin" },
  { value: "students", label: "Học viên" },
  { value: "sessions", label: "Buổi học" },
  { value: "chat", label: "Chat", disabled: true, title: LATER_PHASE_HINT },
  { value: "homework", label: "Bài tập", disabled: true, title: LATER_PHASE_HINT },
  { value: "documents", label: "Tài liệu", disabled: true, title: LATER_PHASE_HINT },
];

const TAB_ID_BASE = "class-detail";

/** `/classes/:id` — header, tabs and the three tabs that already have data. */
export function ClassDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: LiveTab = tabSchema.parse(searchParams.get("tab") ?? undefined);
  const { isOwner } = useCenterContext();
  const klass = useClass(id);
  const today = new Date().toISOString().slice(0, 10);

  function selectTab(next: string) {
    const params = new URLSearchParams(searchParams);
    if (next === "info") {
      params.delete("tab");
    } else {
      params.set("tab", next);
    }
    setSearchParams(params, { replace: true });
  }

  if (klass.isPending) {
    return <HvStateBlock state="loading" title="Đang tải lớp" />;
  }
  if (klass.isError) {
    const notFound = klass.error instanceof ApiError && klass.error.status === 404;
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy lớp" : "Không tải được lớp"}
        description={
          notFound ? "Lớp có thể đã bị xoá hoặc đường dẫn không đúng." : "Thử tải lại trang."
        }
        action={
          <Link
            to="/classes"
            className="font-display text-[13px] font-bold text-mint-600 hover:underline"
          >
            Về danh sách lớp học
          </Link>
        }
      />
    );
  }

  const canWrite = canWriteClass(isOwner, klass.data);

  return (
    <div className="flex flex-col gap-4">
      <ClassDetailHeader klass={klass.data} canWrite={canWrite} />

      <HvSegmented
        variant="tabs"
        idBase={TAB_ID_BASE}
        aria-label="Các mục của lớp"
        options={tabOptions}
        value={tab}
        onValueChange={selectTab}
      />

      <div
        role="tabpanel"
        id={`${TAB_ID_BASE}-panel-${tab}`}
        aria-labelledby={`${TAB_ID_BASE}-tab-${tab}`}
      >
        {tab === "info" ? (
          <ClassInfoTab klass={klass.data} today={today} canWrite={canWrite} isOwner={isOwner} />
        ) : tab === "students" ? (
          <ClassStudentsTab klass={klass.data} />
        ) : (
          <ClassSessionsTab klass={klass.data} today={today} />
        )}
      </div>
    </div>
  );
}
