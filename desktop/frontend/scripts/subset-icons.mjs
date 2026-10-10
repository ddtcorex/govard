// Scans first-party frontend source for Material Symbols icon names and
// writes a subset woff2 + manifest. Runs as `prebuild` so every
// `make frontend` ships only the glyphs the UI actually uses.
//
// Extraction rules:
// - ligature usages (<span ...>settings</span>) are always kept;
// - quoted snake_case literals (icon: maps, getIcon returns) are kept;
// - single-word literals are kept only when allowlisted below, so JS/TS
//   noise ("error" as a status string is a real icon; "return" is not).
// A contributor adding a single-word code-only icon adds it to
// SINGLE_WORD_ICONS; the node test pins the current members.

import { readdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const LIGATURE_RE =
  /material-symbols-outlined[^>]*>\s*([a-z][a-z0-9_]+)\s*</g;
const CODE_LITERAL_RE = /["']([a-z][a-z0-9_]{1,30})["']/g;
// Icon maps name services to glyphs (`caddy: "shield"`) where the value is
// code-only: every value in an icon-named object literal is kept, plus the
// `|| "widgets"` style fallbacks. Without this, single-word map values never
// match the ligature pattern and ship as tofu (caught on globalServiceIcon).
const ICON_MAP_RE = /[A-Za-z]*[Ii]con[A-Za-z0-9]*\s*[:=]\s*\{([^}]*)\}/g;
const FALLBACK_LITERAL_RE = /\|\|\s*["']([a-z][a-z0-9_]{1,30})["']/g;

export const SINGLE_WORD_ICONS = [
  "block",
  "bolt",
  "close",
  "code",
  "dashboard",
  "database",
  "delete",
  "dns",
  "download",
  "email",
  "error",
  "help",
  "history",
  "hub",
  "info",
  "lan",
  "language",
  "memory",
  "monitoring",
  "php",
  "refresh",
  "report",
  "search",
  "settings",
  "stop",
  "sync",
  "terminal",
  "tune",
  "warning",
  "widgets",
];

const SINGLE_WORD_SET = new Set(SINGLE_WORD_ICONS);

export function extractMaterialIconNames(sources) {
  const names = new Set();
  for (const source of sources) {
    const text = String(source ?? "");
    for (const match of text.matchAll(LIGATURE_RE)) {
      names.add(match[1]);
    }
    for (const match of text.matchAll(CODE_LITERAL_RE)) {
      const word = match[1];
      if (word.includes("_") || SINGLE_WORD_SET.has(word)) {
        names.add(word);
      }
    }
    for (const mapMatch of text.matchAll(ICON_MAP_RE)) {
      for (const match of mapMatch[1].matchAll(CODE_LITERAL_RE)) {
        names.add(match[1]);
      }
    }
    for (const match of text.matchAll(FALLBACK_LITERAL_RE)) {
      // Same shape rule as code literals: status strings and service ids
      // ("stopped", "pma", "unknown") ride `||` too and are not glyphs.
      const word = match[1];
      if (word.includes("_") || SINGLE_WORD_SET.has(word)) {
        names.add(word);
      }
    }
  }
  return [...names].sort();
}

const SCAN_DIRS = ["modules", "islands", "ui", "services", "state"];
const SCAN_FILES = ["main.js", "index.html", "preview.html"];
const SCAN_EXTS = new Set([".js", ".tsx"]);

// Exported so the coverage test walks exactly the surface the build scans:
// a name the scanner cannot see is a name the test cannot pin.
export const SCAN_SURFACE = { dirs: SCAN_DIRS, files: SCAN_FILES, exts: [...SCAN_EXTS] };

async function collectSourceTexts(root) {
  const texts = [];
  const walk = async (dir) => {
    for (const entry of await readdir(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        await walk(full);
      } else if (SCAN_EXTS.has(path.extname(entry.name))) {
        texts.push(await readFile(full, "utf8"));
      }
    }
  };
  for (const dir of SCAN_DIRS) {
    await walk(path.join(root, dir));
  }
  for (const file of SCAN_FILES) {
    texts.push(await readFile(path.join(root, file), "utf8"));
  }
  return texts;
}

async function main() {
  const root = fileURLToPath(new URL("..", import.meta.url));
  const { default: subsetFont } = await import("subset-font");
  const { default: fontverter } = await import("fontverter");
  const names = extractMaterialIconNames(await collectSourceTexts(root));
  const fullPath = path.join(root, "assets", "material-symbols.full.woff2");
  const outPath = path.join(root, "assets", "material-symbols.woff2");
  const full = await readFile(fullPath);
  // The hb wasm build cannot decode woff2, so the pipeline runs on sfnt:
  // decompress, subset, shape-verify, recompress.
  const sfnt = Buffer.from(await fontverter.convert(full, "sfnt"));
  // wght 400 is the only weight the UI uses (.material-symbols-outlined
  // sets font-weight: normal); instancing it drops the fvar deltas.
  // keepFeatures rlig: the icon ligatures form under `rlig`, not `liga`
  // (verified by disabling each candidate feature). Default closure is
  // required — noLayoutClosure drops the substitution rules.
  const subset = Buffer.from(
    await subsetFont(sfnt, names.join(" "), {
      targetFormat: "sfnt",
      variationAxes: { wght: 400 },
      keepFeatures: ["rlig"],
      noHinting: true,
    }),
  );
  await verifyLigatures(subset, names);
  const woff2 = Buffer.from(await fontverter.convert(subset, "woff2"));
  await writeFile(outPath, woff2);
  await writeFile(
    path.join(root, "assets", "material-symbols.subset.json"),
    `${JSON.stringify({ icons: names }, null, 2)}\n`,
  );
  console.log(
    `subset-icons: ${names.length} icons, ${full.length} -> ${woff2.length} bytes`,
  );
}

// Every name must shape to exactly one glyph: that is the ligature the UI
// renders. If a name ever stops forming one (font update, over-pruned
// subset), the build fails here instead of shipping tofu icons.
async function verifyLigatures(sfntSubset, names) {
  const hb = await import("harfbuzzjs");
  const bytes = sfntSubset.buffer.slice(
    sfntSubset.byteOffset,
    sfntSubset.byteOffset + sfntSubset.byteLength,
  );
  const font = new hb.Font(new hb.Face(new hb.Blob(bytes)));
  const broken = [];
  for (const name of names) {
    const buffer = new hb.Buffer();
    buffer.addText(name);
    buffer.guessSegmentProperties();
    hb.shape(font, buffer);
    if (buffer.getGlyphInfos().length !== 1) {
      broken.push(name);
    }
  }
  if (broken.length > 0) {
    throw new Error(
      `subset-icons: ligatures lost for ${broken.length} icons: ${broken.join(", ")}`,
    );
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
