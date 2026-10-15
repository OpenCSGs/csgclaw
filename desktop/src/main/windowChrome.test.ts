import assert from "node:assert/strict";
import test from "node:test";
// The options are pure; Electron's Node module export is not used by this helper.
import { windowChromeOptions } from "./windowChrome";
test("macOS chrome keeps native traffic lights inside the application toolbar", () => {
  const options = windowChromeOptions("darwin", true);
  assert.equal(options.titleBarStyle, "hidden");
  assert.deepEqual(options.trafficLightPosition, { x: 16, y: 18 });
  assert.equal(options.titleBarOverlay, undefined);
  assert.equal(options.backgroundColor, "#0c111d");
});
test("Windows and Linux use native overlay controls that follow the theme", () => {
  for (const platform of ["win32", "linux"] as const) {
    const dark = windowChromeOptions(platform, true),
      light = windowChromeOptions(platform, false);
    assert.deepEqual(dark.titleBarOverlay, {
      height: 48,
      color: "#0c111d",
      symbolColor: "#d0d5dd",
    });
    assert.deepEqual(light.titleBarOverlay, {
      height: 48,
      color: "#ffffff",
      symbolColor: "#344054",
    });
    assert.equal(dark.autoHideMenuBar, true);
  }
});
