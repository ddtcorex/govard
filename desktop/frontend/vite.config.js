import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { previewRecordPlugin } from "./preview/vite-plugin-preview.js";

export default defineConfig({
  root: ".",
  base: "./",
  // The "@/*" tsconfig path alias is shadcn's convention and its CLI refuses to
  // resolve relative aliases, so Vite has to know the same mapping.
  resolve: {
    alias: { "@": fileURLToPath(new URL(".", import.meta.url)) },
  },
  // Tailwind v4 runs as a Vite plugin, so there is no PostCSS config and no
  // postcss/autoprefixer devDependency left.
  plugins: [tailwindcss(), previewRecordPlugin()],
  // React 19's automatic JSX runtime, emitted by Vite's own esbuild transform so
  // no @vitejs/plugin-react dependency is needed. Fast Refresh is therefore not
  // available; the islands this serves are preview-only for now.
  esbuild: {
    jsx: "automatic",
  },
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
