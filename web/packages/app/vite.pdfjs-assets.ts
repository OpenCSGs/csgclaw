import { readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import type { Plugin } from "vite";

export function pdfjsAssets(): Plugin {
  const require = createRequire(import.meta.url);
  const root = path.dirname(require.resolve("pdfjs-dist/package.json"));
  const assets = new Map<string, Buffer>();
  for (const directory of ["cmaps", "standard_fonts", "wasm"]) {
    for (const name of readdirSync(path.join(root, directory))) {
      assets.set(`${directory}/${name}`, readFileSync(path.join(root, directory, name)));
    }
  }
  return {
    name: "pdfjs-preview-assets",
    generateBundle() {
      for (const [name, source] of assets) {
        this.emitFile({ type: "asset", fileName: `pdfjs/${name}`, source });
      }
    },
    configureServer(server) {
      const prefix = `${server.config.base}pdfjs/`;
      server.middlewares.use((request, response, next) => {
        const pathname = request.url?.split("?")[0] || "";
        const source = pathname.startsWith(prefix) ? assets.get(pathname.slice(prefix.length)) : undefined;
        if (!source) return next();
        response.setHeader(
          "Content-Type",
          pathname.endsWith(".wasm")
            ? "application/wasm"
            : pathname.endsWith(".js")
              ? "text/javascript"
              : "application/octet-stream",
        );
        response.end(source);
      });
    },
  };
}
