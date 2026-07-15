import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";
import path from "node:path";

const resolvedViteConfig =
  typeof viteConfig === "function" ? viteConfig({ command: "build", mode: "test" }) : viteConfig;

export default mergeConfig(
  resolvedViteConfig,
  defineConfig({
    resolve: {
      alias: [
        {
          find: /^@testing-library\/react$/,
          replacement: path.resolve(__dirname, "tests/testing-library-react.tsx"),
        },
      ],
    },
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: "./tests/setup.ts",
    },
  }),
);
