import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  define: { "import.meta.env.CSGCLAW_UI_LEGACY_CLASSES": "false" },
  build: {
    lib: {
      entry: "src/build.ts",
      formats: ["es"],
      fileName: () => "index.js",
      cssFileName: "styles",
    },
    rolldownOptions: {
      external: [/^react(?:-dom)?(?:\/|$)/, "radix-ui", "lucide-react"],
      output: { comments: { legal: true } },
    },
  },
});
