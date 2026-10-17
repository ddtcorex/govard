// @ts-check
/**
 * The tool palette's data shape and its pure helpers.
 *
 * The palette runs framework tool commands (cache:flush, migrate,
 * composer install...) from preset allowlist entries, without freeform
 * shell. Everything that touches the document lives in the ToolPalette
 * island; what stays here is the part with no element in it:
 *
 * - the run payload builder, which carries a preset ID plus an args map
 *   only. The exec argv is backend-owned (the palette never sends it, so a
 *   hostile frontend cannot upgrade to a shell);
 * - the (op, gen) attribution helpers. Op IDs are stable per project across
 *   reruns, so a terminal event applies only when its generation is still
 *   the current one, and a finished run never flips back to running;
 * - the output buffer, capped like the log panes.
 */

export const TOOL_OUTPUT_EVENT = "tool:output";
export const TOOL_COMPLETED_EVENT = "tool:completed";
export const TOOL_FAILED_EVENT = "tool:failed";

/** Copy shown when no project is selected: the run stays disabled. */
export const NO_PROJECT_GUIDANCE = "Select a project to run tools.";

/** Output buffer cap, matching the log panes. */
export const maxToolOutputLines = 2000;

/**
 * @param {unknown} project
 * @returns {boolean}
 */
export function isToolRunEnabled(project) {
  return String(project ?? "").trim() !== "";
}

/**
 * Builds the run payload from a preset ID and raw field values: the preset
 * ID plus an args map with blank values dropped. The payload has no argv
 * and no command line, ever.
 * @param {unknown} presetId
 * @param {Record<string, unknown>} [values={}]
 * @returns {{ presetId: string, args: Record<string, string> }}
 */
export function buildToolRunPayload(presetId, values = {}) {
  /** @type {Record<string, string>} */
  const args = {};
  for (const [name, raw] of Object.entries(values || {})) {
    const value = String(raw ?? "").trim();
    if (value !== "") {
      args[name] = value;
    }
  }
  return { presetId: String(presetId ?? "").trim(), args };
}

/**
 * The current generation for an op is the newest history gen seen for it.
 * History entries carry (opId, gen), so the palette learns the generation
 * the backend admitted and applies terminal events only for that one.
 * @param {Array<{ opId?: unknown, gen?: unknown }>} history
 * @param {unknown} opId
 * @returns {number}
 */
export function currentToolGeneration(history, opId) {
  const id = String(opId ?? "");
  let gen = 0;
  for (const entry of history || []) {
    if (String(entry?.opId ?? "") !== id) continue;
    const value = Number(entry?.gen ?? 0);
    if (Number.isFinite(value) && value > gen) {
      gen = value;
    }
  }
  return gen;
}

/**
 * @param {string} event
 * @returns {string} "success" for tool:completed, "failed" otherwise.
 */
export function toolTerminalOutcome(event) {
  return event === TOOL_COMPLETED_EVENT ? "success" : "failed";
}

/**
 * Applies one terminal event to a tracked run, idempotently keyed on
 * (op, gen): a terminal for another op or a stale generation is ignored,
 * and a finished run never flips back to running. A run whose generation
 * is still unknown (gen 0, history refresh in flight) adopts the first
 * terminal's generation: the backend emits exactly one terminal per run
 * before it frees the op lock, so no older terminal can arrive after a
 * fresh start.
 * @param {{ opId: string, gen: number, finished: boolean, outcome: string }} run
 * @param {{ opId?: unknown, gen?: unknown, event?: unknown }} payload
 */
export function applyToolTerminalEvent(run, payload) {
  if (run.finished) {
    return run;
  }
  if (String(payload?.opId ?? "") !== run.opId) {
    return run;
  }
  const gen = Number(payload?.gen ?? 0);
  if (run.gen !== 0 && gen !== run.gen) {
    return run;
  }
  return {
    ...run,
    gen: run.gen === 0 && Number.isFinite(gen) ? gen : run.gen,
    finished: true,
    outcome: toolTerminalOutcome(String(payload?.event ?? "")),
  };
}

/**
 * Appends one tool:output batch to the buffer for the tracked op only;
 * output for any other op is dropped. The buffer keeps the last N lines.
 * @param {string[]} lines
 * @param {{ opId?: unknown, text?: unknown }} payload
 * @param {unknown} opId
 * @param {number} [max=maxToolOutputLines]
 * @returns {string[]}
 */
export function applyToolOutputBatch(lines, payload, opId, max = maxToolOutputLines) {
  if (String(payload?.opId ?? "") !== String(opId ?? "")) {
    return lines;
  }
  const appended = [...lines, ...String(payload?.text ?? "").split("\n")];
  const cap = Number(max) > 0 ? Number(max) : maxToolOutputLines;
  return appended.length > cap ? appended.slice(appended.length - cap) : appended;
}
