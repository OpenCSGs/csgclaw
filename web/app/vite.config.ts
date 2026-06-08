import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import checker from "vite-plugin-checker";
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
    chunkSizeWarningLimit: 800,
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
      "/api": "http://127.0.0.1:18080",
      "/healthz": "http://127.0.0.1:18080",
    },
  },
}));
