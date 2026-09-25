// @ts-check

/**
 * Line-scans a generated bindings service file for every exported function's
 * @returns {$CancellablePromise<T>} JSDoc tag. This is the D4 mechanism: the
 * generic default comes from the generated bindings themselves, never a
 * hand-authored table.
 * @param {string} source
 * @returns {{name: string, type: string}[]}
 */
export function parseServiceReturnTypes(source) {
  const lines = source.split("\n");
  const out = [];
  let pendingType = null;
  const returnsRe = /@returns\s*\{\$CancellablePromise<(.+)>\}/;
  const exportRe = /^export function (\w+)\(/;
  for (const line of lines) {
    const returnsMatch = line.match(returnsRe);
    if (returnsMatch) {
      pendingType = returnsMatch[1];
      continue;
    }
    const exportMatch = line.match(exportRe);
    if (exportMatch && pendingType !== null) {
      out.push({ name: exportMatch[1], type: pendingType });
      pendingType = null;
    }
  }
  return out;
}

/**
 * Line-scans a generated bindings service file for the method id each exported
 * function calls. The id regex deliberately does NOT require a closing paren
 * after the digits: `$Call.ByID(1866001717, projectQuery)` forwards an argument,
 * and a `/ByID\((\d+)\)/` form would silently drop all 37 such methods in this
 * tree while still finding the 23 zero-argument ones.
 * @param {string} source
 * @returns {{name: string, id: string}[]}
 */
export function parseServiceMethodIds(source) {
  const out = [];
  let currentName = null;
  for (const line of source.split("\n")) {
    const exportMatch = line.match(/^export function (\w+)\(/);
    if (exportMatch) {
      currentName = exportMatch[1];
      continue;
    }
    const callMatch = line.match(/\$Call\.ByID\((\d+)/);
    if (callMatch && currentName) {
      out.push({ name: currentName, id: callMatch[1] });
      currentName = null;
    }
  }
  return out;
}

/**
 * Manual overrides for a return type the mechanical rules below cannot
 * express. Empty today; the hybrid mechanism D4/D-hybrid describes is
 * "derived, with overrides only where derivation is insufficient" - add an
 * entry here only when classifyReturnType's fallback fires for a real route.
 * @type {Record<string, {kind: string, value?: any, model?: string}>}
 */
export const MANUAL_OVERRIDES = {};

/**
 * @param {string} type
 * @returns {{kind: "void"} | {kind: "slice"} | {kind: "value", value: any} | {kind: "model", model: string}}
 */
export function classifyReturnType(type) {
  if (MANUAL_OVERRIDES[type]) {
    return MANUAL_OVERRIDES[type];
  }
  if (type === "void") {
    return { kind: "void" };
  }
  if (type.endsWith("[]")) {
    return { kind: "slice" };
  }
  if (type === "string") {
    return { kind: "value", value: "" };
  }
  if (type === "number") {
    return { kind: "value", value: 0 };
  }
  if (type === "boolean") {
    return { kind: "value", value: false };
  }
  if (type.startsWith("$models.")) {
    return { kind: "model", model: type.slice("$models.".length) };
  }
  // Unrecognized shape (e.g. a bare Record<string, any>): fail soft with a
  // null default rather than throwing, so one odd signature does not break
  // every other route's generic default. A future case that needs a real
  // default belongs in MANUAL_OVERRIDES above, not a parser special case.
  return { kind: "value", value: null };
}
