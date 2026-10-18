// Shared sanitizer for every UI-bound string (toasts, status lines,
// progress messages). Mirrors sanitizeUIText in
// internal/desktop/output_text.go: strip ANSI escape sequences and orphan
// style fragments, drop control characters except newline and tab, drop
// lone surrogates (the JS form of invalid UTF-8), and truncate over-long
// input with a truncation marker. A max <= 0 disables truncation.

const ANSI_ESCAPE = /\x1b\[[0-?]*[ -/]*[@-~]/g;
const ORPHAN_ANSI_STYLE = /\[\d{1,3}(?:;\d{1,3})*m/g;
const CONTROL_CHARS = /[\x00-\x08\x0b\x0c\x0d-\x1f\x7f]/g;
const LONE_SURROGATES =
  /[\ud800-\udbff](?![\udc00-\udfff])|(?:[^\ud800-\udbff]|^)[\udc00-\udfff]/g;

export const TRUNCATED_MARKER = "…(truncated)";

export function sanitizeUIText(value, max) {
  let text = String(value ?? "");
  if (text === "") {
    return "";
  }
  text = text.replace(ANSI_ESCAPE, "").replace(ORPHAN_ANSI_STYLE, "");
  text = text.replace(LONE_SURROGATES, (match) =>
    match.length > 1 ? match[0] : "",
  );
  text = text.replace(CONTROL_CHARS, "");
  if (max > 0) {
    const runes = [...text];
    if (runes.length > max) {
      const marker = [...TRUNCATED_MARKER];
      if (max <= marker.length) {
        return marker.slice(0, max).join("");
      }
      return runes.slice(0, max - marker.length).join("") + TRUNCATED_MARKER;
    }
  }
  return text;
}

// sanitizeRichConfirmHTML prepares rich markup for innerHTML surfaces (the
// confirm dialog renders line breaks and muted notes, so textContent is not
// an option). Everything is escaped first; only <br> and a <small> carrying
// nothing but a plain class attribute are restored. Any other tag or
// attribute — handlers, sources, styles — stays escaped text.
export function sanitizeRichConfirmHTML(value) {
  const clean = sanitizeUIText(value, 0);
  if (clean === "") {
    return "";
  }
  let out = clean
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
  out = out.replace(/&lt;br\s*\/?&gt;/gi, "<br>");
  out = out.replace(
    /&lt;(\/?)small((?:\s+class=(?:"([\w\s-]+)"|'([\w\s-]+)'))?)\s*&gt;/gi,
    (_match, close, _attrs, doubleQuoted, singleQuoted) => {
      const cls = doubleQuoted ?? singleQuoted;
      return `<${close}small${cls ? ` class="${cls}"` : ""}>`;
    },
  );
  return out;
}
