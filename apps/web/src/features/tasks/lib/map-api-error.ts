import type { FieldValues, Path, UseFormReturn } from "react-hook-form";

import { ApiError, toApiError } from "@/lib/api/errors";
import { isKanbanError, type KanbanError, type KanbanErrorEntity } from "@/lib/kanban";

/**
 * Translates a rejected API call into the `KanbanDataSource` port's error
 * union. The lib never inspects HTTP status codes itself (see
 * `src/lib/kanban/README.md` "Ports") — this is the one place that does.
 * `entity` disambiguates 404s since the HTTP layer alone can't tell a
 * missing column from a missing task.
 */
export function mapApiError(error: unknown, entity: KanbanErrorEntity = "task"): KanbanError {
  const apiError = error instanceof ApiError ? error : toApiError(error);
  switch (apiError.status) {
    case 404:
      return { kind: "not-found", entity };
    case 409:
      return { kind: "conflict" };
    case 403:
      return { kind: "forbidden" };
    case 422:
      return { kind: "validation", fields: apiError.fields ?? {} };
    default:
      return { kind: "unknown", cause: apiError };
  }
}

const GENERIC_ERROR_MESSAGE = "Đã xảy ra lỗi, vui lòng thử lại.";

/**
 * Maps a `KanbanError` (not a raw `ApiError` — see `mapApiError` above) onto
 * a react-hook-form instance. This is a different mapping than
 * `@/lib/forms/use-api-form-errors` (which expects `ApiError` directly):
 * task-form-modal submits through the `KanbanDataSource` adapter, so by the
 * time an error reaches the form it has already been translated once.
 */
export function applyKanbanFormError<T extends FieldValues>(
  form: UseFormReturn<T>,
  error: unknown,
): void {
  if (!isKanbanError(error)) {
    form.setError("root", { type: "server", message: GENERIC_ERROR_MESSAGE });
    return;
  }
  switch (error.kind) {
    case "validation": {
      const entries = Object.entries(error.fields);
      if (entries.length === 0) {
        form.setError("root", { type: "server", message: "Dữ liệu không hợp lệ." });
        return;
      }
      const known = new Set(Object.keys(form.getValues()));
      for (const [field, message] of entries) {
        if (known.has(field)) {
          form.setError(field as Path<T>, { type: "server", message });
        } else {
          form.setError("root", { type: "server", message });
        }
      }
      return;
    }
    case "conflict":
      form.setError("root", {
        type: "server",
        message: "Dữ liệu vừa thay đổi, vui lòng tải lại.",
      });
      return;
    case "forbidden":
      form.setError("root", {
        type: "server",
        message: "Bạn không có quyền thực hiện thao tác này.",
      });
      return;
    case "not-found":
      form.setError("root", {
        type: "server",
        message:
          error.entity === "task" ? "Công việc không còn tồn tại." : "Cột không còn tồn tại.",
      });
      return;
    case "unknown":
      form.setError("root", { type: "server", message: GENERIC_ERROR_MESSAGE });
  }
}

/**
 * Vietnamese toast copy for a `KanbanError` surfaced outside a form.
 * `fallback` names the action that failed ("Không chuyển được việc, …") for
 * errors that carry no reason of their own (network, 5xx), so the toast
 * still tells the user what just went wrong.
 */
export function kanbanErrorToastMessage(
  error: unknown,
  fallback: string = GENERIC_ERROR_MESSAGE,
): string {
  if (!isKanbanError(error)) {
    return fallback;
  }
  switch (error.kind) {
    case "validation":
      return Object.values(error.fields)[0] ?? "Dữ liệu không hợp lệ.";
    case "conflict":
      return "Bảng đã thay đổi, đang tải lại.";
    case "forbidden":
      return "Bạn không có quyền thực hiện thao tác này.";
    case "not-found":
      return error.entity === "task" ? "Công việc không còn tồn tại." : "Cột không còn tồn tại.";
    case "unknown":
      return fallback;
  }
}
