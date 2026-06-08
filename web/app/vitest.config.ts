import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

const resolvedViteConfig =
  typeof viteConfig === "function" ? viteConfig({ command: "build", mode: "test" }) : viteConfig;

export default mergeConfig(
  resolvedViteConfig,
  defineConfig({
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: "./tests/setup.ts",
    },
  }),
);
