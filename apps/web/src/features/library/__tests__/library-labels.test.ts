import { describe, expect, it } from "vitest";

import { materialFormatLabel, templateStatus } from "../lib/library-labels";

describe("materialFormatLabel", () => {
  it.each([
    ["https://youtu.be/abc", "YouTube"],
    ["https://www.youtube.com/watch?v=abc", "YouTube"],
    ["https://drive.google.com/file/d/1/view", "Drive"],
    ["https://cdn.example.com/files/bai-1.PDF", "PDF"],
    ["https://cdn.example.com/slides.pptx?dl=1", "PPTX"],
    ["https://cdn.example.com/audio.mp3", "MP3"],
    ["https://example.com/lesson", "Link"],
    ["not a url", "Link"],
    ["", "—"],
    [null, "—"],
  ])("%s → %s", (url, label) => {
    expect(materialFormatLabel(url)).toBe(label);
  });
});

describe("templateStatus", () => {
  const v = (status: "draft" | "published" | "archived") => ({ id: status, version_no: 1, status });

  it("is active once any version is published", () => {
    expect(templateStatus({ versions: [v("archived"), v("published"), v("draft")] })).toBe(
      "active",
    );
  });
  it("is a draft with only drafts or no versions", () => {
    expect(templateStatus({ versions: [v("draft"), v("archived")] })).toBe("draft");
    expect(templateStatus({ versions: [] })).toBe("draft");
  });
  it("is stopped when every version is archived", () => {
    expect(templateStatus({ versions: [v("archived")] })).toBe("archived");
  });
});
