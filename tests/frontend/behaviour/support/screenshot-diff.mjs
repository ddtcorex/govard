// @ts-check
import { readFileSync } from "node:fs";
import { decodePNG } from "./png.mjs";

/**
 * Compares two PNG buffers pixel by pixel.
 *
 * `threshold` is the largest per-channel delta that still counts as
 * anti-aliasing noise (Tailwind v3 to v4 moves sub-pixel rendering slightly even
 * when nothing about the design changed). What the caller cares about is
 * `passesThreshold`; `diffPixelCount` and `maxDelta` are what a human reads when
 * it fails.
 * @param {Buffer} bufA
 * @param {Buffer} bufB
 * @param {{threshold: number}} opts
 */
export function diffPNG(bufA, bufB, { threshold }) {
  if (bufA.equals(bufB)) {
    return { maxDelta: 0, diffPixelCount: 0, passesThreshold: true };
  }
  const a = decodePNG(bufA);
  const b = decodePNG(bufB);
  if (a.width !== b.width || a.height !== b.height) {
    return { maxDelta: 255, diffPixelCount: a.width * a.height, passesThreshold: false, reason: "dimension mismatch" };
  }
  let maxDelta = 0;
  let diffPixelCount = 0;
  for (let i = 0; i < a.pixels.length; i += 4) {
    const delta = Math.max(
      Math.abs(a.pixels[i] - b.pixels[i]),
      Math.abs(a.pixels[i + 1] - b.pixels[i + 1]),
      Math.abs(a.pixels[i + 2] - b.pixels[i + 2]),
      Math.abs(a.pixels[i + 3] - b.pixels[i + 3]),
    );
    if (delta > 0) diffPixelCount++;
    maxDelta = Math.max(maxDelta, delta);
  }
  return { maxDelta, diffPixelCount, passesThreshold: maxDelta <= threshold };
}

export async function diffPNGFiles(pathA, pathB, opts) {
  return diffPNG(readFileSync(pathA), readFileSync(pathB), opts);
}
