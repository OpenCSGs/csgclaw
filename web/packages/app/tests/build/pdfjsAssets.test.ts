import { pdfjsAssets } from "../../vite.pdfjs-assets";

it("ships local CJK maps, fallback fonts, decoders, and their licenses", () => {
  const plugin = pdfjsAssets();
  const emitFile = vi.fn();
  if (typeof plugin.generateBundle !== "function") throw new Error("missing asset build hook");
  Reflect.apply(plugin.generateBundle, { emitFile }, []);
  const names = emitFile.mock.calls.map(([asset]) => asset.fileName);
  expect(names).toEqual(
    expect.arrayContaining([
      "pdfjs/cmaps/UniGB-UCS2-H.bcmap",
      "pdfjs/cmaps/LICENSE",
      "pdfjs/standard_fonts/LiberationSans-Regular.ttf",
      "pdfjs/standard_fonts/LICENSE_LIBERATION",
      "pdfjs/wasm/openjpeg.wasm",
      "pdfjs/wasm/LICENSE_OPENJPEG",
    ]),
  );
  expect(emitFile.mock.calls.every(([asset]) => asset.source.byteLength > 0)).toBe(true);
});

it("serves the same assets under the development base path without exposing other files", () => {
  const plugin = pdfjsAssets();
  const use = vi.fn();
  if (typeof plugin.configureServer !== "function") throw new Error("missing development asset hook");
  Reflect.apply(plugin.configureServer, undefined, [{ config: { base: "/tenant/" }, middlewares: { use } }]);
  const middleware = use.mock.calls[0][0];
  const response = { setHeader: vi.fn(), end: vi.fn() };
  const next = vi.fn();
  middleware({ url: "/tenant/pdfjs/wasm/openjpeg.wasm?cache=1" }, response, next);
  expect(response.setHeader).toHaveBeenCalledWith("Content-Type", "application/wasm");
  expect(response.end).toHaveBeenCalledOnce();
  expect(response.end.mock.calls[0][0].subarray(0, 4).toString("hex")).toBe("0061736d");
  expect(next).not.toHaveBeenCalled();
  middleware({ url: "/tenant/pdfjs/../package.json" }, response, next);
  expect(next).toHaveBeenCalledOnce();
});
