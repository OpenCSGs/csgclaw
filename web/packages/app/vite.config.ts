import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import checker from "vite-plugin-checker";
import { defineConfig } from "vite";
import type { PluginOption } from "vite";
import path from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";
import { pdfjsAssets } from "./vite.pdfjs-assets";

const staticDistDir = path.resolve(__dirname, "../../static-dist");

const keepStaticDistPlaceholderPlugin: PluginOption = {
  name: "keep-static-dist-placeholder",
  closeBundle(): void {
    mkdirSync(staticDistDir, { recursive: true });
    writeFileSync(path.join(staticDistDir, ".gitkeep"), "");
  },
};

const appVendorModulePaths = [
  "@floating-ui/",
  "@radix-ui/",
  "@tanstack/",
  "dompurify/",
  "lucide-react/",
  "marked/",
  "radix-ui/",
  "react/",
  "react-dom/",
  "react-router/",
  "react-router-dom/",
  "scheduler/",
  "zustand/",
];

function manualChunks(id: string): string | undefined {
  return appVendorModulePaths.some((modulePath) => id.includes(`/node_modules/${modulePath}`))
    ? "vendor-app"
    : undefined;
}

export default defineConfig(({ command }) => ({
  plugins: [
    tailwindcss(),
    react(),
    pdfjsAssets(),
    keepStaticDistPlaceholderPlugin,
    command === "serve" &&
      checker({
        typescript: true,
        eslint: {
          lintCommand: 'eslint "./src/**/*.{ts,tsx}" "./tests/**/*.{ts,tsx}"',
          useFlatConfig: true,
        },
        overlay: {
          initialIsOpen: false,
        },
        terminal: true,
      }),
  ].filter(Boolean) as PluginOption[],
  base: "./",
  publicDir: "public",
  define: { "import.meta.env.CSGCLAW_UI_LEGACY_CLASSES": "true" },
  resolve: {
    conditions: ["csgclaw-source", "module", "browser", "development|production"],
    dedupe: ["react", "react-dom"],
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  build: {
    outDir: staticDistDir,
    emptyOutDir: true,
    assetsDir: "assets",
    sourcemap: false,
    // Allow the existing PPT preview bundle without changing module execution order.
    chunkSizeWarningLimit: 1500,
    rolldownOptions: {
      input: {
        app: path.resolve(__dirname, "index.html"),
        "sse-shared-worker": path.resolve(__dirname, "src/shared/realtime/sseSharedWorker.ts"),
      },
      output: {
        entryFileNames: (chunk) => (chunk.name === "sse-shared-worker" ? "[name].js" : "assets/[name]-[hash].js"),
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash][extname]",
        manualChunks,
      },
    },
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      "/api": { target: "http://127.0.0.1:18080", changeOrigin: false },
      "/healthz": "http://127.0.0.1:18080",
    },
  },
}));
