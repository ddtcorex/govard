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
