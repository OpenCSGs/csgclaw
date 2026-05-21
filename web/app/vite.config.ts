import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import type { PluginOption } from "vite";
import path from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";

const keepStaticDistPlaceholderPlugin: PluginOption = {
  name: "keep-static-dist-placeholder",
  closeBundle(): void {
    const outDir = path.resolve(__dirname, "../static-dist");
    mkdirSync(outDir, { recursive: true });
    writeFileSync(path.join(outDir, ".gitkeep"), "");
  },
};

export default defineConfig({
  plugins: [react(), keepStaticDistPlaceholderPlugin],
  base: "./",
  publicDir: "public",
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  build: {
    outDir: "../static-dist",
    emptyOutDir: true,
    assetsDir: "assets",
    sourcemap: false,
    // Mermaid is lazy-loaded only for diagram messages; keep size warnings focused on app chunks.
    chunkSizeWarningLimit: 650,
    rollupOptions: {
      input: {
        app: path.resolve(__dirname, "index.html"),
        "sse-shared-worker": path.resolve(__dirname, "src/shared/realtime/sseSharedWorker.ts"),
      },
      output: {
        entryFileNames: (chunk) => (chunk.name === "sse-shared-worker" ? "[name].js" : "assets/[name]-[hash].js"),
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash][extname]",
      },
    },
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:18080",
      "/healthz": "http://127.0.0.1:18080",
    },
  },
});
