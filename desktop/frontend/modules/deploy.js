// Read-only deploy helpers for the DeployPanel island (M1). Every string
// that reaches the panel goes through sanitizeUIText, mirroring the Go
// sanitizeUIText in internal/desktop/output_text.go, so host paths and
// engine output render as plain text.

import { sanitizeUIText } from "../utils/sanitize.js";

/** Empty-target guidance: zero releases is a normal state, not an error. */
export const emptyReleasesGuidance = () =>
  "No releases found on this remote yet. Run a deploy once, then come back here to read its status.";

/** Release rows the panel renders: revision, branch and env stay visible. */
export const normalizeDeployReleases = (raw) => {
  const list = Array.isArray(raw)
    ? raw
    : Array.isArray(raw?.Releases)
      ? raw.Releases
      : Array.isArray(raw?.releases)
        ? raw.releases
        : [];
  return list.map((entry) => {
    const source = entry && typeof entry === "object" ? entry : {};
    const pick = (...keys) => {
      for (const key of keys) {
        const value = source[key];
        if (value !== undefined && value !== null && value !== "") {
          return value;
        }
      }
      return "";
    };
    return {
      release: sanitizeUIText(pick("Release", "release"), 128),
      revision: sanitizeUIText(pick("Revision", "revision"), 512),
      branch: sanitizeUIText(pick("Branch", "branch"), 128),
      status: sanitizeUIText(pick("Status", "status"), 128),
      createdAt: sanitizeUIText(pick("CreatedAt", "createdAt"), 128),
      createdBy: sanitizeUIText(pick("CreatedBy", "createdBy"), 128),
      live: Boolean(source.Live ?? source.live ?? false),
      foreign: Boolean(source.Foreign ?? source.foreign ?? false),
    };
  });
};

/** Plan and check output panes render through this, never raw. */
export const sanitizeDeployText = (value) => sanitizeUIText(value, 0);

/**
 * The check DTO's ProbePerformed flag drives this note: a symlink-strategy
 * check runs a transient probe write, so the panel discloses it instead of
 * presenting the check as purely observational.
 */
export const probeDisclosureNote = (check) => {
  const performed = Boolean(check?.probePerformed ?? check?.ProbePerformed);
  if (!performed) {
    return "";
  }
  return "Note: this check performed a transient probe write to verify atomic swaps on the target.";
};

/** The remote picker gates every fetch: no remote, no call. */
export const deployFetchBlocked = (project, remote) => {
  if (!String(project || "").trim() || !String(remote || "").trim()) {
    return "Select a remote before loading deploy data.";
  }
  return "";
};
