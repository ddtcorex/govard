import test from "node:test";
import assert from "node:assert/strict";
import { deflateSync } from "node:zlib";
import { decodePNG } from "./behaviour/support/png.mjs";
import { diffPNG } from "./behaviour/support/screenshot-diff.mjs";

/** Encodes a tiny 2x2 RGBA PNG by hand, so the test needs no external fixture. */
function makeTinyPNG(pixels /* 2x2 array of [r,g,b,a] */) {
  const width = 2, height = 2;
  const sig = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
  function chunk(type, data) {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const typeBuf = Buffer.from(type);
    // CRC is not validated by decodePNG; zero is fine for this fixture.
    const crc = Buffer.alloc(4);
    return Buffer.concat([len, typeBuf, data, crc]);
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // color type: RGBA
  const rows = [];
  for (let y = 0; y < height; y++) {
    rows.push(Buffer.from([0])); // filter type: none
    for (let x = 0; x < width; x++) {
      rows.push(Buffer.from(pixels[y * width + x]));
    }
  }
  const raw = Buffer.concat(rows);
  const idat = deflateSync(raw);
  return Buffer.concat([sig, chunk("IHDR", ihdr), chunk("IDAT", idat), chunk("IEND", Buffer.alloc(0))]);
}

test("decodePNG round-trips a hand-built RGBA PNG", () => {
  const png = makeTinyPNG([
    [255, 0, 0, 255], [0, 255, 0, 255],
    [0, 0, 255, 255], [255, 255, 255, 255],
  ]);
  const decoded = decodePNG(png);
  assert.equal(decoded.width, 2);
  assert.equal(decoded.height, 2);
  assert.deepEqual([...decoded.pixels.slice(0, 4)], [255, 0, 0, 255]);
});

test("diffPNG reports identical for byte-identical images", () => {
  const png = makeTinyPNG([
    [10, 10, 10, 255], [20, 20, 20, 255],
    [30, 30, 30, 255], [40, 40, 40, 255],
  ]);
  const result = diffPNG(png, png, { threshold: 2 });
  assert.equal(result.maxDelta, 0);
  assert.equal(result.passesThreshold, true);
});

test("diffPNG flags a pixel delta above the anti-aliasing threshold", () => {
  const before = makeTinyPNG([
    [10, 10, 10, 255], [20, 20, 20, 255],
    [30, 30, 30, 255], [40, 40, 40, 255],
  ]);
  const after = makeTinyPNG([
    [10, 10, 10, 255], [70, 20, 20, 255],
    [30, 30, 30, 255], [40, 40, 40, 255],
  ]);
  const result = diffPNG(before, after, { threshold: 2 });
  assert.equal(result.maxDelta, 50);
  assert.equal(result.passesThreshold, false);
});

// Sub-threshold anti-aliasing noise must not fail the gate, or the migration
// gate would be unusable on a real 1440x900 screenshot.
test("diffPNG tolerates a delta at the threshold but counts the pixels", () => {
  const before = makeTinyPNG([
    [10, 10, 10, 255], [20, 20, 20, 255],
    [30, 30, 30, 255], [40, 40, 40, 255],
  ]);
  const after = makeTinyPNG([
    [10, 10, 10, 255], [22, 20, 20, 255],
    [30, 30, 30, 255], [40, 40, 40, 255],
  ]);
  const result = diffPNG(before, after, { threshold: 2 });
  assert.equal(result.maxDelta, 2);
  assert.equal(result.diffPixelCount, 1);
  assert.equal(result.passesThreshold, true);
});

// The decoder's contract is "what Chrome produces", and it must refuse anything
// else loudly instead of returning pixels that mean nothing.
test("decodePNG rejects a bit depth it cannot read", () => {
  const png = makeTinyPNG([
    [1, 2, 3, 255], [4, 5, 6, 255],
    [7, 8, 9, 255], [10, 11, 12, 255],
  ]);
  png[24] = 16; // IHDR bit depth, the 9th byte of the IHDR data at offset 16+8
  assert.throws(() => decodePNG(png), /unsupported PNG/);
});

test("diffPNG refuses images whose dimensions differ", () => {
  const small = makeTinyPNG([
    [1, 1, 1, 255], [2, 2, 2, 255],
    [3, 3, 3, 255], [4, 4, 4, 255],
  ]);
  const wide = (() => {
    // A 3x2 RGBA PNG built the same way, so the dimensions really differ.
    const sig = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
    const chunk = (type, data) => {
      const len = Buffer.alloc(4);
      len.writeUInt32BE(data.length);
      return Buffer.concat([len, Buffer.from(type), data, Buffer.alloc(4)]);
    };
    const ihdr = Buffer.alloc(13);
    ihdr.writeUInt32BE(3, 0);
    ihdr.writeUInt32BE(2, 4);
    ihdr[8] = 8;
    ihdr[9] = 6;
    const rows = [];
    for (let y = 0; y < 2; y++) {
      rows.push(Buffer.from([0]));
      for (let x = 0; x < 3; x++) rows.push(Buffer.from([1, 2, 3, 255]));
    }
    return Buffer.concat([sig, chunk("IHDR", ihdr), chunk("IDAT", deflateSync(Buffer.concat(rows))), chunk("IEND", Buffer.alloc(0))]);
  })();
  const result = diffPNG(small, wide, { threshold: 2 });
  assert.equal(result.passesThreshold, false);
  assert.equal(result.reason, "dimension mismatch");
});
