import { defineConfig } from "vite";
import { previewRecordPlugin } from "./preview/vite-plugin-preview.js";

export default defineConfig({
  root: ".",
  base: "./",
  plugins: [previewRecordPlugin()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
  },
  server: {
    port: 5173,
    strictPort: true,
  },
});
