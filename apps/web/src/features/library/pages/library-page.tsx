import { Navigate, useNavigate, useSearchParams } from "react-router";

import { HvSegmented, HvStateBlock, type HvSegmentedOption } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";

import { ExercisesBank } from "../components/exercises-bank";
import { MaterialsBank } from "../components/materials-bank";
import { TemplateCards } from "../components/template-cards";

export type LibraryTab = "templates" | "materials" | "exercises";

const tabOptions: HvSegmentedOption<LibraryTab>[] = [
  { value: "templates", label: "Chương trình mẫu" },
  { value: "materials", label: "Ngân hàng nội dung" },
  { value: "exercises", label: "Ngân hàng bài tập" },
];

const TAB_ID_BASE = "library";

const TAB_PATH: Record<LibraryTab, string> = {
  templates: "/library",
  materials: "/library/materials",
  exercises: "/library/exercises",
};

/**
 * `/library` · `/library/materials` · `/library/exercises` — the center's
 * program-template hub plus the two shared banks it only ever references
 * (never copies). Each tab is its own route so the sidebar's Kho học liệu
 * group can deep-link straight into one, and so `useNavActive`'s
 * longest-prefix match lights exactly one nav entry per tab.
 */
export function LibraryPage({ tab }: { tab: LibraryTab }) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { has, isResolved } = useCenterContext();
  const canEdit = has("library.edit");

  // `/library` used to carry every tab behind `?tab=`; keep old links and
  // bookmarks working by redirecting once to the tab's own route, and keep
  // any other query param (e.g. a search term) that rode along with it.
  if (tab === "templates") {
    const legacyTab = searchParams.get("tab");
    if (legacyTab !== null) {
      const target =
        legacyTab === "materials" || legacyTab === "exercises" ? TAB_PATH[legacyTab] : "/library";
      const rest = new URLSearchParams(searchParams);
      rest.delete("tab");
      const query = rest.toString();
      return <Navigate to={query ? `${target}?${query}` : target} replace />;
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Kho học liệu</h1>
        <p className="mt-1 text-[14px] text-ink-500">
          Ba cấp: ngân hàng nội dung, ngân hàng bài tập và chương trình mẫu. Chương trình mẫu chỉ
          tham chiếu tới hai ngân hàng — không giữ bản sao.
        </p>
      </div>

      <HvSegmented
        variant="tabs"
        idBase={TAB_ID_BASE}
        aria-label="Các mục của kho học liệu"
        options={tabOptions}
        value={tab}
        onValueChange={(next) => void navigate(TAB_PATH[next])}
      />

      <div
        role="tabpanel"
        id={`${TAB_ID_BASE}-panel-${tab}`}
        aria-labelledby={`${TAB_ID_BASE}-tab-${tab}`}
      >
        {!isResolved ? (
          <HvStateBlock state="loading" title="Đang tải kho học liệu" />
        ) : tab === "templates" ? (
          <TemplateCards canEdit={canEdit} />
        ) : tab === "materials" ? (
          <MaterialsBank canEdit={canEdit} />
        ) : (
          <ExercisesBank canEdit={canEdit} />
        )}
      </div>
    </div>
  );
}
