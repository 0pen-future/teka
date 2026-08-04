/**
 * Copies text to the clipboard, trying the async Clipboard API first and
 * falling back to a hidden, selectable `<textarea>` plus
 * `document.execCommand("copy")` for non-secure contexts (plain http, some
 * in-app webviews) where `navigator.clipboard` is unavailable. Returns
 * whether the copy actually succeeded so callers can toast honestly instead
 * of assuming success — copying text is not the same as sending it.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the textarea fallback below — some browsers reject
      // the Clipboard API outside a secure context or a user gesture even
      // when the API itself is present.
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "0";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);

  let succeeded = false;
  try {
    succeeded = document.execCommand("copy");
  } catch {
    succeeded = false;
  } finally {
    document.body.removeChild(textarea);
  }
  return succeeded;
}
