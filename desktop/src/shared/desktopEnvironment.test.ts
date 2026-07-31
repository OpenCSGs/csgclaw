import assert from "node:assert/strict";
import test from "node:test";
import {
  CSGClawExecutableName,
  csgclawExecutableForPlatform,
  desktopArchForGoArch,
  DesktopArchitecture,
  DesktopPlatform,
  goArchForDesktopArch,
  GoArchitecture,
  goOSForDesktopPlatform,
  GoOperatingSystem,
} from "./desktopEnvironment";

test("maps Electron platforms to CSGClaw executable names and Go operating systems", () => {
  assert.equal(
    csgclawExecutableForPlatform(DesktopPlatform.Windows),
    CSGClawExecutableName.Windows,
  );
  assert.equal(
    csgclawExecutableForPlatform(DesktopPlatform.MacOS),
    CSGClawExecutableName.Posix,
  );
  assert.equal(
    csgclawExecutableForPlatform(DesktopPlatform.Linux),
    CSGClawExecutableName.Posix,
  );
  assert.equal(goOSForDesktopPlatform(DesktopPlatform.Windows), GoOperatingSystem.Windows);
  assert.equal(goOSForDesktopPlatform(DesktopPlatform.MacOS), GoOperatingSystem.MacOS);
  assert.equal(goOSForDesktopPlatform(DesktopPlatform.Linux), GoOperatingSystem.Linux);
});

test("maps Electron and Go architecture names in both directions", () => {
  assert.equal(goArchForDesktopArch(DesktopArchitecture.X64), GoArchitecture.AMD64);
  assert.equal(goArchForDesktopArch(DesktopArchitecture.ARM64), GoArchitecture.ARM64);
  assert.equal(desktopArchForGoArch(GoArchitecture.AMD64), DesktopArchitecture.X64);
  assert.equal(desktopArchForGoArch(GoArchitecture.ARM64), DesktopArchitecture.ARM64);
});
