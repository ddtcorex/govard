// @ts-check
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Dev-only, and skipped entirely outside `vite dev` (`apply: "serve"`), so it
 * never reaches `dist/`:
 *
 * 1. A `govard:record` handler on Vite's own HMR channel appends every captured
 *    call to `preview/fixtures/<GOVARD_PREVIEW_RECORD_NAME>.json`, or to
 *    `preview/fixtures/<Service>.json` when no name is set (see fixtureNameFor).
 * 2. With `GOVARD_PREVIEW_RECORD=1` it injects a script tag that installs the
 *    recording loader before main.js runs, so neither the committed index.html
 *    nor main.js changes (spec D2; the loaders stay the only seam).
 *
 * The HMR channel is the carrier, not an HTTP POST. Measured 2026-09-25 in the
 * real desktop window: the page's own origin is the Wails asset server, whose
 * dev-server proxy forwards the request but drops the body, so a POST middleware
 * saw `content-length: 0` and an empty payload 28 times and recorded nothing.
 * Vite's HMR WebSocket is the one channel that proxy must forward intact,
 * because HMR itself depends on it, and a custom event is a first-class Vite API
 * (`import.meta.hot.send` on the page, `server.hot.on` here) with no extra
 * dependency.
 *
 * The fixtures directory comes from Vite's own resolved root, not from
 * `process.cwd()` (which depends on where the dev server was started) and not
 * from `import.meta.url` (the config is loaded through Vite's config loader).
 */
/**
 * The fixture file an entry belongs in. GOVARD_PREVIEW_RECORD_NAME wins, so a
 * recording session can be aimed at the module a migration task is about
 * ("GOVARD_PREVIEW_RECORD_NAME=metrics" lands in fixtures/metrics.json, the name
 * installFixtures("metrics") reads). Otherwise one file per service: a single
 * dump mixing every route is not usable as a fixture, and the app has no idea
 * which file a call belongs to.
 * @param {{service?: string}} entry
 * @param {string | undefined} recordName
 * @returns {string} empty when there is nothing to name the file after
 */
export function fixtureNameFor(entry, recordName) {
  const requested = String(recordName ?? "").trim();
  if (requested) return requested;
  return String(entry?.service ?? "").trim();
}

export function previewRecordPlugin() {
  return {
    name: "govard-preview-record",
    apply: "serve",
    configureServer(server) {
      const fixturesDir = join(server.config.root, "preview", "fixtures");
      const hot = server.hot ?? server.ws;
      hot.on("govard:record", (entry) => {
        if (!entry || typeof entry !== "object") return;
        const name = fixtureNameFor(entry, process.env.GOVARD_PREVIEW_RECORD_NAME);
        if (!name) {
          server.config.logger.warn(
            "[preview] record entry without a service was dropped; set GOVARD_PREVIEW_RECORD_NAME to name the file",
          );
          return;
        }
        if (!existsSync(fixturesDir)) mkdirSync(fixturesDir, { recursive: true });
        const file = join(fixturesDir, `${name}.json`);
        const existing = existsSync(file) ? JSON.parse(readFileSync(file, "utf8")) : [];
        existing.push(entry);
        writeFileSync(file, JSON.stringify(existing, null, 2) + "\n");
        server.config.logger.info(
          `[preview] recorded ${entry.service}.${entry.method} -> preview/fixtures/${name}.json (${existing.length} entries)`,
        );
      });
    },
    transformIndexHtml(html) {
      if (process.env.GOVARD_PREVIEW_RECORD !== "1") return html;
      return html.replace(
        "</head>",
        '<script type="module" src="/preview/record-bootstrap.js"></script></head>',
      );
    },
  };
}
