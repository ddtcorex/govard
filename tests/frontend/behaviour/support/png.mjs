// @ts-check
import { inflateSync } from "node:zlib";

/**
 * Minimal PNG decoder scoped to what Chrome's Page.captureScreenshot produces:
 * 8-bit depth, non-interlaced, color type 2 (RGB) or 6 (RGBA). CRC is not
 * verified. Anything else fails loudly rather than silently comparing garbage.
 * @param {Buffer} buffer
 * @returns {{width: number, height: number, pixels: Uint8ClampedArray}}
 */
export function decodePNG(buffer) {
  let offset = 8; // skip the 8-byte signature
  let width = 0, height = 0, bitDepth = 0, colorType = 0;
  const idatChunks = [];
  while (offset < buffer.length) {
    const len = buffer.readUInt32BE(offset);
    const type = buffer.toString("ascii", offset + 4, offset + 8);
    const data = buffer.subarray(offset + 8, offset + 8 + len);
    if (type === "IHDR") {
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      bitDepth = data[8];
      colorType = data[9];
      const interlace = data[12];
      if (bitDepth !== 8 || (colorType !== 2 && colorType !== 6) || interlace !== 0) {
        throw new Error(
          `decodePNG: unsupported PNG (bitDepth=${bitDepth}, colorType=${colorType}, interlace=${interlace}); only 8-bit non-interlaced RGB/RGBA is supported`,
        );
      }
    } else if (type === "IDAT") {
      idatChunks.push(data);
    } else if (type === "IEND") {
      break;
    }
    offset += 12 + len;
  }
  const channels = colorType === 6 ? 4 : 3;
  const raw = inflateSync(Buffer.concat(idatChunks));
  const stride = width * channels;
  const pixels = new Uint8ClampedArray(width * height * 4);
  let rawOffset = 0;
  const prevRow = new Uint8ClampedArray(stride);
  for (let y = 0; y < height; y++) {
    const filterType = raw[rawOffset];
    rawOffset += 1;
    const row = raw.subarray(rawOffset, rawOffset + stride);
    rawOffset += stride;
    const unfiltered = new Uint8ClampedArray(stride);
    for (let x = 0; x < stride; x++) {
      const a = x >= channels ? unfiltered[x - channels] : 0;
      const b = prevRow[x];
      const c = x >= channels ? prevRow[x - channels] : 0;
      let value = row[x];
      if (filterType === 1) value += a;
      else if (filterType === 2) value += b;
      else if (filterType === 3) value += Math.floor((a + b) / 2);
      else if (filterType === 4) {
        const p = a + b - c;
        const pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c);
        value += pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
      }
      unfiltered[x] = value & 0xff;
    }
    for (let x = 0; x < width; x++) {
      const srcIdx = x * channels;
      const dstIdx = (y * width + x) * 4;
      pixels[dstIdx] = unfiltered[srcIdx];
      pixels[dstIdx + 1] = unfiltered[srcIdx + 1];
      pixels[dstIdx + 2] = unfiltered[srcIdx + 2];
      pixels[dstIdx + 3] = channels === 4 ? unfiltered[srcIdx + 3] : 255;
    }
    prevRow.set(unfiltered);
  }
  return { width, height, pixels };
}
