import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite-plus";

export default defineConfig({
  // The app window: one page.
  root: "src/renderer",
  base: "./",
  plugins: [react(), tailwindcss()],
  build: { outDir: "../../dist/renderer", emptyOutDir: true },
  // The main process and the preload script. Electron loads CommonJS preloads
  // in sandboxed windows, so both are CommonJS.
  pack: {
    entry: {
      "main/index": "src/main/index.ts",
      "preload/index": "src/preload/index.ts",
    },
    outDir: "dist",
    format: "cjs",
    platform: "node",
    // Everything is bundled except Electron itself, so the app ships no
    // node_modules.
    deps: { neverBundle: ["electron"], onlyBundle: false },
    clean: false,
  },
  test: {
    root: ".",
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
