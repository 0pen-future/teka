import DOMPurify, { type Config } from "dompurify";

/**
 * The HTML subset a task description may contain. It mirrors the API's
 * bluemonday policy (`apps/api/internal/features/tasks/description.go`):
 * the server is the authority, this is defense in depth for what the
 * browser renders and for what the editor is seeded with.
 */
export const SANITIZE_CONFIG = {
  ALLOWED_TAGS: ["p", "br", "strong", "em", "u", "s", "ul", "ol", "li", "a"],
  ALLOWED_ATTR: ["href", "rel", "target"],
  ALLOWED_URI_REGEXP: /^(?:https?:|mailto:)/i,
  ALLOW_DATA_ATTR: false,
  KEEP_CONTENT: true,
} satisfies Config;

/**
 * Link attributes written the way bluemonday writes them, so a description
 * that round-trips through the editor unchanged serializes to the same
 * string the server stored and a no-op save is a no-op request. Only a
 * fully qualified link (a scheme with a host) opens in a new tab and needs
 * noopener; a mailto: link keeps rel only.
 */
const FULLY_QUALIFIED = /^https?:\/\//i;
const LINK_REL = "nofollow noreferrer";
const LINK_REL_NEW_TAB = "nofollow noreferrer noopener";

/**
 * An isolated DOMPurify instance so the hook below cannot leak into another
 * caller's sanitizer. Created on first use: the module may be evaluated
 * before a document exists (lazy chunks, tests importing the lib alone).
 */
let purifier: ReturnType<typeof DOMPurify> | undefined;

function getPurifier() {
  if (!purifier) {
    purifier = DOMPurify(window);
    purifier.addHook("afterSanitizeAttributes", (node) => {
      if (node.tagName !== "A") return;
      if (!node.hasAttribute("href")) {
        // A link whose URL failed the scheme check keeps its text only.
        node.removeAttribute("target");
        node.removeAttribute("rel");
        return;
      }
      if (FULLY_QUALIFIED.test(node.getAttribute("href") ?? "")) {
        node.setAttribute("rel", LINK_REL_NEW_TAB);
        node.setAttribute("target", "_blank");
      } else {
        node.setAttribute("rel", LINK_REL);
        node.removeAttribute("target");
      }
    });
  }
  return purifier;
}

/** Sanitizes a description to the allowed subset; whitespace-only input becomes "". */
export function sanitizeDescription(html: string): string {
  const trimmed = html.trim();
  if (trimmed === "") return "";
  return getPurifier().sanitize(trimmed, SANITIZE_CONFIG).trim();
}

/**
 * A document that starts with a tag, closing tag, comment or doctype. Plain
 * text that merely begins with "<" ("<3 you", "< 5 phút") is still wrapped
 * like any other legacy row (same rule as the API's `markupStart`).
 */
const MARKUP_START = /^<[A-Za-z!/]/;

function escapeText(text: string): string {
  return text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

/**
 * What the app accepts from the API before showing or editing it. Rows the
 * migration has not rewritten (or a client that still sends plain text) are
 * wrapped the way migration 000023 wraps them: one escaped paragraph with
 * line breaks as `<br>`. The result is sanitized either way.
 */
export function normalizeIncoming(raw: string): string {
  const trimmed = raw.trim();
  if (trimmed === "") return "";
  if (MARKUP_START.test(trimmed)) return sanitizeDescription(trimmed);
  return sanitizeDescription(`<p>${escapeText(trimmed).replace(/\r?\n/g, "<br>")}</p>`);
}

function parseBody(html: string): HTMLElement {
  // A parsed document is inert: no scripts run, nothing is fetched.
  return new DOMParser().parseFromString(html, "text/html").body;
}

/**
 * Length in code points of the text the API measures ("&amp;" is one
 * character, tags and line breaks are none), so the form's 2000 limit
 * trips on exactly the input the server would reject.
 */
export function plainTextLength(html: string): number {
  if (html === "") return 0;
  return Array.from(parseBody(html).textContent ?? "").length;
}

/** One-line text for previews: blocks and line breaks collapse to single spaces. */
export function textFromHtml(html: string): string {
  if (html === "") return "";
  const body = parseBody(html);
  for (const boundary of body.querySelectorAll("p, li, br")) {
    boundary.after(" ");
  }
  return (body.textContent ?? "").replace(/\s+/g, " ").trim();
}
