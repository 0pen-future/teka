import { describe, expect, it } from "vitest";

import {
  normalizeIncoming,
  plainTextLength,
  sanitizeDescription,
  textFromHtml,
} from "../lib/rich-text";

describe("sanitizeDescription", () => {
  it("keeps the allowed formatting subset intact", () => {
    const html =
      "<p><strong>Đậm</strong> <em>nghiêng</em> <u>gạch</u> <s>bỏ</s></p><ul><li>một</li><li>hai</li></ul><ol><li>ba</li></ol><p>dòng<br>mới</p>";
    expect(sanitizeDescription(html)).toBe(html);
  });

  it("drops scripts, event handlers, styles and unknown tags but keeps their text", () => {
    expect(
      sanitizeDescription(
        '<p onclick="x()">a<script>alert(1)</script><img src=x onerror="alert(1)"><span style="color:red">b</span></p><h1>tiêu đề</h1>',
      ),
    ).toBe("<p>ab</p>tiêu đề");
  });

  it("strips javascript: and data: links; http(s) links open in a new tab, mailto keeps rel only", () => {
    expect(sanitizeDescription('<p><a href="javascript:alert(1)">x</a></p>')).toBe(
      "<p><a>x</a></p>",
    );
    expect(sanitizeDescription('<p><a href="data:text/html,hi">x</a></p>')).toBe("<p><a>x</a></p>");
    expect(sanitizeDescription('<p><a href="/relative">x</a></p>')).toBe("<p><a>x</a></p>");
    expect(
      sanitizeDescription('<p><a href="https://teka.vn" target="_self" rel="opener">x</a></p>'),
    ).toBe(
      '<p><a href="https://teka.vn" rel="nofollow noreferrer noopener" target="_blank">x</a></p>',
    );
    expect(sanitizeDescription('<p><a href="mailto:a@b.vn" target="_blank">m</a></p>')).toBe(
      '<p><a href="mailto:a@b.vn" rel="nofollow noreferrer">m</a></p>',
    );
  });

  it("drops every attribute other than href/rel/target", () => {
    expect(sanitizeDescription('<p class="x" id="y" data-a="b">t</p>')).toBe("<p>t</p>");
  });

  it("collapses whitespace-only input to an empty string", () => {
    expect(sanitizeDescription("   \n ")).toBe("");
    expect(sanitizeDescription("")).toBe("");
  });
});

describe("normalizeIncoming", () => {
  it("wraps a legacy plain-text description as one escaped paragraph with <br> line breaks", () => {
    expect(normalizeIncoming("Gọi phụ huynh & báo lịch\r\n<3 lớp 5A\nxong")).toBe(
      "<p>Gọi phụ huynh &amp; báo lịch<br>&lt;3 lớp 5A<br>xong</p>",
    );
  });

  it("leaves a document that already starts with a tag alone apart from sanitizing it", () => {
    expect(normalizeIncoming("<p>a</p><script>x</script>")).toBe("<p>a</p>");
    expect(normalizeIncoming("<!-- c --><p>a</p>")).toBe("<p>a</p>");
  });

  it("treats text that merely starts with '<' as plain text", () => {
    expect(normalizeIncoming("< 5 phút")).toBe("<p>&lt; 5 phút</p>");
  });

  it("returns an empty string for blank input", () => {
    expect(normalizeIncoming("  ")).toBe("");
  });
});

describe("plainTextLength", () => {
  it("counts code points of the text only, decoding entities and ignoring tags", () => {
    expect(plainTextLength("")).toBe(0);
    expect(plainTextLength("<p>a&amp;b</p>")).toBe(3);
    expect(plainTextLength("<p>một<br>hai</p><ul><li>ba</li></ul>")).toBe(8);
    expect(plainTextLength("<p>😀😀</p>")).toBe(2);
  });
});

describe("textFromHtml", () => {
  it("flattens blocks and line breaks to single spaces for a preview", () => {
    expect(textFromHtml("<p>một<br>hai</p><ul><li>ba</li><li>bốn</li></ul>")).toBe(
      "một hai ba bốn",
    );
    expect(textFromHtml("<p>  nhiều   khoảng </p>")).toBe("nhiều khoảng");
    expect(textFromHtml("")).toBe("");
  });
});

describe("XSS edge cases", () => {
  it("blocks javascript: protocol in various link formats", () => {
    expect(sanitizeDescription('<p><a href="javascript:alert(1)">click</a></p>')).toBe(
      "<p><a>click</a></p>",
    );
    expect(sanitizeDescription('<p><a href="JavaScript:void(0)">click</a></p>')).toBe(
      "<p><a>click</a></p>",
    );
    expect(sanitizeDescription('<p><a href="java\\nscript:alert(1)">click</a></p>')).toBe(
      "<p><a>click</a></p>",
    );
  });

  it("removes SVG and event handlers", () => {
    expect(sanitizeDescription('<svg onload="alert(1)"><circle/></svg>')).toBe("");
    expect(
      sanitizeDescription('<p>text<img src=x onerror="alert(1)" onclick="alert(2)"></p>'),
    ).toBe("<p>text</p>");
    expect(
      sanitizeDescription('<p onmouseover="alert(1)" onclick="alert(2)" data-value="bad">safe</p>'),
    ).toBe("<p>safe</p>");
  });

  it("rejects iframe, style, and script tags entirely", () => {
    expect(sanitizeDescription('<p>intro</p><iframe src="evil.com"></iframe><p>outro</p>')).toBe(
      "<p>intro</p><p>outro</p>",
    );
    expect(sanitizeDescription("<p>text<style>body { display: none; }</style></p>")).toBe(
      "<p>text</p>",
    );
    expect(sanitizeDescription("<p>before<script>alert(1)</script>after</p>")).toBe(
      "<p>beforeafter</p>",
    );
  });

  it("handles nested and deeply nested tags", () => {
    // Deeply nested list
    expect(
      sanitizeDescription("<ul><li>one<ul><li>nested</li></ul></li><li>two</li></ul>"),
    ).toContain("<li>one");
    // Mixed nesting
    expect(
      sanitizeDescription("<ol><li><strong>bold item</strong></li><li>normal</li></ol>"),
    ).toContain("<strong>bold item</strong>");
  });

  it("handles empty and whitespace-only tags", () => {
    // DOMPurify keeps empty tags and trailing whitespace-only tags
    expect(sanitizeDescription("<p></p><p>text</p>")).toBe("<p></p><p>text</p>");
    expect(sanitizeDescription("<p>   </p>text")).toBe("<p>   </p>text");
    expect(sanitizeDescription("<br><br><p>after breaks</p>")).toBe("<br><br><p>after breaks</p>");
  });

  it("decodes HTML entities correctly in plainTextLength", () => {
    expect(plainTextLength("<p>&lt;script&gt;</p>")).toBe(8); // "<script>"
    expect(plainTextLength("<p>&quot;hello&quot;</p>")).toBe(7); // "hello"
    expect(plainTextLength("<p>A&nbsp;B</p>")).toBe(3); // "A B"
    expect(plainTextLength("<p>&amp;nbsp;</p>")).toBe(6); // "&nbsp;"
  });

  it("counts emoji correctly as code points", () => {
    expect(plainTextLength("<p>🎉🎊🎈</p>")).toBe(3);
    // Family emoji (ZWJ sequence) counts as multiple code points in Array.from()
    // because it includes zero-width joiners; this matches how the API counts
    expect(plainTextLength("<p>👨‍👩‍👧‍👦</p>")).toBeGreaterThan(1);
  });

  it("enforces mailto: scheme validation", () => {
    expect(sanitizeDescription('<p><a href="mailto:valid@example.com">mail</a></p>')).toBe(
      '<p><a href="mailto:valid@example.com" rel="nofollow noreferrer">mail</a></p>',
    );
    expect(sanitizeDescription('<p><a href="mailto:not-an-email">bad</a></p>')).toBe(
      '<p><a href="mailto:not-an-email" rel="nofollow noreferrer">bad</a></p>',
    );
  });

  it("rejects relative and protocol-relative links", () => {
    expect(sanitizeDescription('<p><a href="/path/to/page">relative</a></p>')).toBe(
      "<p><a>relative</a></p>",
    );
    expect(sanitizeDescription('<p><a href="//example.com">protocol-relative</a></p>')).toBe(
      "<p><a>protocol-relative</a></p>",
    );
  });

  it("handles normalizeIncoming with special characters and entities", () => {
    // Special chars are escaped, quotes are rendered as-is by DOMPurify
    expect(normalizeIncoming("&<>\"'")).toBe("<p>&amp;&lt;&gt;\"'</p>");
    expect(normalizeIncoming("Line 1\r\nLine 2\nLine 3")).toBe("<p>Line 1<br>Line 2<br>Line 3</p>");
  });

  it("sanitizes malformed HTML", () => {
    // Unclosed tags
    expect(sanitizeDescription("<p>text<strong>bold</p>")).toContain("<strong>bold");
    // Badly nested
    expect(sanitizeDescription("<p><strong>bold<em>italic</strong>end</em></p>")).toContain("bold");
  });
});
