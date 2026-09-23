import { Link, useParams, useSearchParams } from "react-router";
import { z } from "zod";

import { HvSegmented, HvStateBlock, type HvSegmentedOption } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";

import { ClassChatPanel } from "../components/class-chat-panel";
import { ClassDetailHeader } from "../components/class-detail-header";
import { ClassDocumentsTab } from "../components/class-documents-tab";
import { ClassHomeworkTab } from "../components/class-homework-tab";
import { ClassInfoTab } from "../components/class-info-tab";
import { ClassSessionsTab } from "../components/class-sessions-tab";
import { ClassStudentsTab } from "../components/class-students-tab";
import { useClass } from "../hooks/use-classes";
import { canWriteClass } from "../lib/class-permissions";

const tabs = ["info", "students", "sessions", "chat", "homework", "documents"] as const;
type Tab = (typeof tabs)[number];

const tabSchema = z.enum(tabs).catch("info");

const tabOptions: HvSegmentedOption<string>[] = [
  { value: "info", label: "Thông tin" },
  { value: "students", label: "Học viên" },
  { value: "sessions", label: "Buổi học" },
  { value: "chat", label: "Chat" },
  { value: "homework", label: "Bài tập" },
  { value: "documents", label: "Tài liệu" },
];

const TAB_ID_BASE = "class-detail";

/** `/classes/:id` — header and the six tabs of a class. */
export function ClassDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: Tab = tabSchema.parse(searchParams.get("tab") ?? undefined);
  const { isOwner, has } = useCenterContext();
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
          <ClassInfoTab
            klass={klass.data}
            today={today}
            canWrite={canWrite}
            isOwner={isOwner}
            canReadAudit={has("audit.read")}
          />
        ) : tab === "students" ? (
          <ClassStudentsTab klass={klass.data} />
        ) : tab === "sessions" ? (
          <ClassSessionsTab klass={klass.data} today={today} />
        ) : tab === "chat" ? (
          <ClassChatPanel
            klass={klass.data}
            canPost={has("class_messages.post")}
            isOwner={isOwner}
          />
        ) : tab === "homework" ? (
          <ClassHomeworkTab klass={klass.data} />
        ) : (
          <ClassDocumentsTab klass={klass.data} />
        )}
      </div>
    </div>
  );
}
