import assert from "node:assert/strict";
import test from "node:test";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import {
  windowsAppUserModelID,
  windowsTaskbarAppDetails,
} from "./windowsTaskbar";

test("Squirrel taskbar details keep a stable application identity", () => {
  assert.deepEqual(
    windowsTaskbarAppDetails(
      DesktopPlatform.Windows,
      false,
      "C:\\icons\\light.ico",
    ),
    {
      appId: windowsAppUserModelID,
      appIconPath: "C:\\icons\\light.ico",
      appIconIndex: 0,
    },
  );
  assert.deepEqual(
    windowsTaskbarAppDetails(
      DesktopPlatform.Windows,
      false,
      "C:\\icons\\dark.ico",
    ),
    {
      appId: windowsAppUserModelID,
      appIconPath: "C:\\icons\\dark.ico",
      appIconIndex: 0,
    },
  );
});

test("Microsoft Store packages retain their manifest taskbar identity", () => {
  assert.equal(
    windowsTaskbarAppDetails(
      DesktopPlatform.Windows,
      true,
      "C:\\icons\\light.ico",
    ),
    undefined,
  );
});

test("taskbar details are only produced for Windows with an icon", () => {
  assert.equal(
    windowsTaskbarAppDetails(DesktopPlatform.MacOS, false, "/icons/light.ico"),
    undefined,
  );
  assert.equal(
    windowsTaskbarAppDetails(DesktopPlatform.Windows, false, ""),
    undefined,
  );
});
