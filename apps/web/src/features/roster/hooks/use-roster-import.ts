import { useMutation, useQueryClient } from "@tanstack/react-query";

import { downloadTemplate, importRoster, type ImportRosterInput } from "../api/imports-api";
import { classesKeys, contactsKeys, enrollmentsKeys, studentsKeys } from "./roster-keys";

/** Filename Excel shows the operator; the server sends the same one. */
const TEMPLATE_FILENAME = "teka-import-mau.xlsx";

/**
 * Downloads the blank workbook and hands it to the browser's save flow. The
 * object URL is revoked immediately after the synthetic click — the blob
 * holds the whole file in memory and nothing needs it once the download has
 * started.
 */
export function useDownloadTemplate() {
  return useMutation({
    mutationFn: downloadTemplate,
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = TEMPLATE_FILENAME;
      link.click();
      URL.revokeObjectURL(url);
    },
  });
}

/**
 * Runs one workbook, as a check or for real. A committed import creates rows
 * across all four roster entities, so it invalidates all four — the class
 * list and the student list are the two screens the operator lands on next.
 * A check writes nothing and therefore invalidates nothing.
 */
export function useImportRoster() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ImportRosterInput) => importRoster(input),
    onSuccess: (report) => {
      if (!report.committed) {
        return;
      }
      for (const keys of [classesKeys, studentsKeys, contactsKeys, enrollmentsKeys]) {
        void queryClient.invalidateQueries({ queryKey: keys.all });
      }
    },
  });
}
