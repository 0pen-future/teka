export type CsvCell = string | number | null | undefined;

/**
 * Prototype-exact CSV encoding: a UTF-8 BOM so Excel autodetects the
 * encoding, semicolon separators (Vietnamese Excel treats comma as the
 * decimal mark), and every cell quoted with `""`-escaping. Rows join with
 * bare `\n`, matching the prototype byte-for-byte.
 */
export function toCsv(rows: readonly (readonly CsvCell[])[]): string {
  return (
    "\uFEFF" +
    rows
      .map((row) => row.map((cell) => `"${String(cell ?? "").replace(/"/g, '""')}"`).join(";"))
      .join("\n")
  );
}

/** Triggers a browser download; content building stays testable via `toCsv`. */
export function downloadCsv(fileName: string, rows: readonly (readonly CsvCell[])[]): void {
  const url = URL.createObjectURL(new Blob([toCsv(rows)], { type: "text/csv;charset=utf-8" }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  URL.revokeObjectURL(url);
}
