// @ts-check
import { installFakeBindingsLoader } from "./fake-loaders.js";
import { installFakeTransport } from "./fake-transport.js";

/**
 * The Phase 0 gate's verdict (docs/superpowers/specs/2026-09-25-wails3-preview-spike-findings.md):
 * Candidate A won, because only the transport seam exercises the generated model
 * path, so only it can catch a fixture whose keys do not match the Go JSON tags.
 * Both candidates are fully implemented behind this one switch point: flipping
 * this constant is the whole change, and nothing else has to be rewritten.
 * @type {"loader" | "transport"}
 */
export const PREVIEW_SEAM = "transport";

/**
 * Installs the active seam. Both candidates return a restore function (or a
 * promise of one), which the preview ignores today and record mode reuses.
 */
export async function installPreviewSeam() {
  if (PREVIEW_SEAM === "loader") {
    return installFakeBindingsLoader();
  }
  return installFakeTransport();
}
