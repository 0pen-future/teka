import { describe, expect, it } from "vitest";

import { keepWhileSearching } from "../hooks/next-page";

const rows = { pages: ["previous rows"] };

describe("keepWhileSearching", () => {
  it("keeps the previous rows while only the search text changes", () => {
    const placeholder = keepWhileSearching({ query: "an", per_page: 50 });
    expect(placeholder(rows, { queryKey: ["list", { query: "a", per_page: 50 }] })).toBe(rows);
  });

  it("starts blank when the list itself changes", () => {
    const placeholder = keepWhileSearching({ query: "", unenrolled: true, per_page: 50 });
    expect(placeholder(rows, { queryKey: ["list", { query: "", per_page: 50 }] })).toBeUndefined();
  });

  it("starts blank with no previous query", () => {
    expect(keepWhileSearching({ query: "" })(rows, undefined)).toBeUndefined();
  });
});
