import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { useCenter } from "@/features/center";
import { ApiError } from "@/lib/api/errors";

import { ImportErrorTable } from "../components/import-error-table";
import { ImportReportSummary } from "../components/import-report-summary";
import { importRowErrors } from "../api/imports-api";
import { useDownloadTemplate, useImportRoster } from "../hooks/use-roster-import";
import type { ImportErrorsPayload, ImportReport } from "../schemas/import-schemas";

/**
 * Owner-only bulk onboarding: download the template, fill it in Excel, check
 * it, then commit. The check (`dry_run=true`) runs the identical resolution
 * and the identical existence lookups as the commit, so a clean check cannot
 * be followed by a surprise — the two-step flow exists to let the operator
 * see what a 300-row file is about to do before it does it.
 *
 * The role gate here is convenience only. `GET /centers/me` is role-shaped
 * and carries no role flag, so ownership is read by narrowing on the owner
 * body's `members` key; the API's own owner check is the real authorization.
 */
export function RosterImportPage() {
  const { data: center, isPending, isError } = useCenter();

  if (isPending) {
    return <p className="text-[14px] text-ink-400">Đang tải…</p>;
  }
  if (isError || !center) {
    return <p className="text-[14px] text-ink-500">Không tải được thông tin trung tâm.</p>;
  }
  if (!("members" in center)) {
    return (
      <div>
        <ImportPageHeading />
        <HvCard className="mt-[18px]">
          <p className="text-[14px] text-ink-500">
            Chỉ chủ trung tâm mới nhập được dữ liệu từ file Excel. Nhờ chủ trung tâm nhập giúp, hoặc
            tự thêm lớp và học sinh trong màn hình{" "}
            <Link to="/students" className="font-bold text-mint-600">
              Lớp &amp; học sinh
            </Link>
            .
          </p>
        </HvCard>
      </div>
    );
  }

  return <ImportFlow />;
}

function ImportPageHeading() {
  return (
    <div>
      <h1 className="font-display text-[26px] font-extrabold text-ink-900">Nhập từ file Excel</h1>
      <p className="mt-1 text-[13.5px] text-ink-500">
        Tạo hàng loạt lớp, lịch học, phụ huynh, học sinh và ghi danh từ một file. Nhập lại cùng một
        file sẽ không tạo trùng — hệ thống nhận ra những dòng đã có.
      </p>
    </div>
  );
}

/** The owner's four-step flow; separate so the role gate above stays flat. */
function ImportFlow() {
  const [file, setFile] = useState<File | null>(null);
  /** A clean check result, cleared the moment anything invalidates it. */
  const [checked, setChecked] = useState<ImportReport | null>(null);
  const [committed, setCommitted] = useState<ImportReport | null>(null);
  const [rowErrors, setRowErrors] = useState<ImportErrorsPayload | null>(null);
  /** Whole-file or transport failures — a 403, a 409, a bad workbook, a timeout. */
  const [failure, setFailure] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();

  const templateMutation = useDownloadTemplate();
  const importMutation = useImportRoster();
  // Which of the two actions is in flight: the mutation is shared, so the
  // pending state has to be attributed before a spinner can be placed.
  const runningDryRun = importMutation.isPending && importMutation.variables?.dryRun === true;
  const runningCommit = importMutation.isPending && importMutation.variables?.dryRun === false;

  function clearReport() {
    setChecked(null);
    setCommitted(null);
    setRowErrors(null);
    setFailure(null);
  }

  function pickFile(picked: File | null) {
    setFile(picked);
    // A "hợp lệ" badge sitting next to a file that was never checked is the
    // one dangerous bug on this page — it would invite committing unchecked
    // data. Any file change resets the whole result.
    clearReport();
  }

  function run(dryRun: boolean) {
    if (!file) {
      return;
    }
    clearReport();
    importMutation.mutate(
      { file, dryRun },
      {
        onSuccess: (report) => {
          if (report.committed) {
            setCommitted(report);
          } else {
            setChecked(report);
          }
        },
        onError: (error) => {
          // A commit can fail on conflicts a clean check never saw — someone
          // else created the same class in between — so the check result is
          // dropped either way and the operator re-checks.
          setChecked(null);
          const payload = importRowErrors(error);
          if (payload) {
            setRowErrors(payload);
            return;
          }
          setFailure(
            error instanceof ApiError ? error.message : "Không nhập được file. Thử lại sau.",
          );
        },
      },
    );
  }

  function startOver() {
    setFile(null);
    clearReport();
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <ImportPageHeading />

      <HvCard>
        <p className="font-display text-[16px] font-bold text-ink-900">1. Tải file mẫu</p>
        <p className="mt-0.5 mb-3 text-[13px] text-ink-400">
          File mẫu có 2 sheet: <strong>Lop</strong> và <strong>HocSinh</strong>, kèm dòng ví dụ. Giữ
          nguyên tên sheet và dòng tiêu đề.
        </p>
        <HvButton
          variant="secondary"
          size="sm"
          onClick={() => templateMutation.mutate()}
          disabled={templateMutation.isPending}
        >
          {templateMutation.isPending ? "Đang tải…" : "Tải file mẫu"}
        </HvButton>
        {templateMutation.isError ? (
          <p className="mt-2 text-[13px] text-coral-600">
            {templateMutation.error instanceof ApiError
              ? templateMutation.error.message
              : "Không tải được file mẫu."}
          </p>
        ) : null}
      </HvCard>

      <HvCard>
        <p className="font-display text-[16px] font-bold text-ink-900">2. Chọn file đã điền</p>
        <p className="mt-0.5 mb-3 text-[13px] text-ink-400">Định dạng .xlsx, tối đa 2 MB.</p>
        <input
          ref={fileInputRef}
          type="file"
          accept=".xlsx"
          aria-label="Chọn file Excel"
          onChange={(event) => pickFile(event.target.files?.[0] ?? null)}
          className="block w-full text-[13.5px] text-ink-500 file:mr-3 file:min-h-11 file:cursor-pointer file:rounded-[var(--radius-md)] file:border-0 file:bg-cream-200 file:px-4 file:font-display file:text-[14px] file:font-bold file:text-ink-700 hover:file:bg-line-200"
        />
      </HvCard>

      <div className="flex flex-wrap items-center gap-3">
        <HvButton
          variant="secondary"
          onClick={() => run(true)}
          disabled={!file || importMutation.isPending}
        >
          {runningDryRun ? "Đang kiểm tra…" : "Kiểm tra"}
        </HvButton>
        <HvButton
          onClick={() => run(false)}
          // Only a clean check unlocks the commit, and only once: after a
          // successful import the same file would just report every row as
          // already existing.
          disabled={!file || !checked || Boolean(committed) || importMutation.isPending}
        >
          {runningCommit ? "Đang nhập dữ liệu…" : "Nhập dữ liệu"}
        </HvButton>
        {file ? (
          <HvButton
            variant="ghost"
            size="sm"
            onClick={startOver}
            disabled={importMutation.isPending}
          >
            Chọn file khác
          </HvButton>
        ) : null}
      </div>

      {runningCommit ? (
        <p className="text-[13px] text-ink-400">
          Đang ghi dữ liệu, đừng đóng trang. File lớn có thể mất khoảng một phút.
        </p>
      ) : null}

      {failure ? (
        <HvCard variant="flat" className="text-[13.5px] text-coral-600">
          {failure}
        </HvCard>
      ) : null}

      {rowErrors ? <ImportErrorTable payload={rowErrors} /> : null}

      {checked ? <ImportReportSummary report={checked} /> : null}

      {committed ? (
        <div className="flex flex-col gap-3">
          <ImportReportSummary report={committed} />
          <div className="flex flex-wrap gap-3">
            <HvButton
              size="sm"
              onClick={() => {
                void navigate("/students");
              }}
            >
              Xem lớp &amp; học sinh
            </HvButton>
            <HvButton variant="ghost" size="sm" onClick={startOver}>
              Nhập file khác
            </HvButton>
          </div>
        </div>
      ) : null}
    </div>
  );
}
