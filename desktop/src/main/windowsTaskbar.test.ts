import assert from "node:assert/strict";
import test from "node:test";
import type { NativeImage } from "electron";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import {
  windowsAppUserModelID,
  windowsTaskbarAppDetails,
  WindowsTaskbarIcon,
  type WindowsTaskbarAppDetails,
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

function fakeIcon(empty = false): NativeImage {
  return {
    isEmpty: () => empty,
  } as NativeImage;
}

function fakeWindow() {
  return {
    destroyed: false,
    visible: true,
    taskbarVisibility: [] as boolean[],
    icons: [] as NativeImage[],
    details: [] as WindowsTaskbarAppDetails[],
    isDestroyed() {
      return this.destroyed;
    },
    isVisible() {
      return this.visible;
    },
    setSkipTaskbar(skip: boolean) {
      this.taskbarVisibility.push(skip);
    },
    setIcon(image: NativeImage | string) {
      this.icons.push(image as NativeImage);
    },
    setAppDetails(details: WindowsTaskbarAppDetails) {
      this.details.push(details);
    },
  };
}

test("caches the selected theme before opening and restores it for a replacement window", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false);
  const light = fakeIcon();
  const dark = fakeIcon();
  taskbar.select(light, "light.ico");
  taskbar.select(dark, "dark.ico");
  assert.equal(taskbar.image, dark);
  const first = fakeWindow();
  taskbar.apply(first);
  first.destroyed = true;
  const replacement = fakeWindow();
  taskbar.apply(replacement);
  assert.deepEqual(first.icons, [dark]);
  assert.deepEqual(replacement.icons, [dark]);
  assert.equal(replacement.details[0]?.appIconPath, "dark.ico");
});

test("changes the entire live icon while preserving Squirrel identity and relaunch ownership", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false);
  const window = fakeWindow();
  const light = fakeIcon();
  const dark = fakeIcon();
  for (const [image, iconPath] of [
    [light, "light.ico"],
    [dark, "dark.ico"],
    [light, "light.ico"],
  ] as const) {
    taskbar.select(image, iconPath);
    taskbar.apply(window);
  }
  assert.deepEqual(window.icons, [light, dark, light]);
  assert.deepEqual(window.taskbarVisibility, [true, false, true, false]);
  assert.deepEqual(
    window.details.map((details) => details.appIconPath),
    ["light.ico", "dark.ico", "light.ico"],
  );
  for (const details of window.details) {
    assert.equal(details.appId, windowsAppUserModelID);
    assert.equal("relaunchCommand" in details, false);
    assert.equal("relaunchDisplayName" in details, false);
  }
});

test("MSIX changes presentation without assigning Squirrel taskbar properties", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, true, () => {
    throw new Error("MSIX must not touch Squirrel shortcuts");
  });
  const window = fakeWindow();
  for (const dark of [false, true]) {
    taskbar.select(fakeIcon(), dark ? "dark.ico" : "light.ico");
    taskbar.apply(window);
  }
  assert.equal(window.icons.length, 2);
  assert.deepEqual(window.details, []);
  assert.deepEqual(window.taskbarVisibility, []);
});

test("coalesces repeated theme events but reapplies the icon when shown again", () => {
  let syncs = 0;
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false, () => {
    syncs++;
    return "persistent-light.ico";
  });
  const window = fakeWindow();
  taskbar.select(fakeIcon(), "light.ico");
  taskbar.apply(window);
  taskbar.select(fakeIcon(), "light.ico");
  taskbar.apply(window);
  assert.equal(window.icons.length, 1);
  assert.equal(syncs, 1);
  assert.equal(window.details[0]?.appIconPath, "persistent-light.ico");
  taskbar.apply(window, true);
  assert.equal(window.icons.length, 2);
  assert.deepEqual(window.taskbarVisibility, []);
  window.destroyed = true;
  taskbar.apply(window, true);
  assert.equal(window.icons.length, 2);
});

test("refreshes the final taskbar icon after rapid theme changes settle", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false);
  const window = fakeWindow();
  const light = fakeIcon();
  const dark = fakeIcon();

  taskbar.select(light, "light.ico");
  taskbar.apply(window);
  taskbar.select(dark, "dark.ico");
  taskbar.apply(window);
  taskbar.select(light, "light.ico");
  taskbar.apply(window);
  taskbar.refresh(window);

  assert.deepEqual(window.icons, [light, dark, light, light]);
  assert.equal(window.details.at(-1)?.appIconPath, "light.ico");
  assert.deepEqual(
    window.taskbarVisibility,
    [true, false, true, false, true, false],
  );
});

test("ignores invalid icons without losing the last valid selection", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false);
  const window = fakeWindow();
  taskbar.apply(window);
  assert.deepEqual(window.icons, []);
  const light = fakeIcon();
  taskbar.select(light, "light.ico");
  taskbar.select(fakeIcon(true), "dark.ico");
  taskbar.select(fakeIcon(), "");
  taskbar.apply(window);
  assert.deepEqual(window.icons, [light]);
});

test("does not call Windows presentation APIs on other platforms", () => {
  for (const platform of [DesktopPlatform.MacOS, DesktopPlatform.Linux]) {
    const taskbar = new WindowsTaskbarIcon(platform, false);
    const window = fakeWindow();
    taskbar.select(fakeIcon(), "dark.ico");
    taskbar.apply(window, true);
    assert.equal(taskbar.image, undefined);
    assert.deepEqual(window.icons, []);
    assert.deepEqual(window.details, []);
  }
});

test("retries a failed native update instead of caching it as applied", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false);
  const window = fakeWindow();
  taskbar.select(fakeIcon(), "light.ico");
  taskbar.apply(window);
  const applyDetails = window.setAppDetails;
  window.setAppDetails = () => {
    throw new Error("Shell unavailable");
  };
  taskbar.select(fakeIcon(), "dark.ico");
  assert.throws(() => taskbar.apply(window), /Shell unavailable/);
  assert.deepEqual(window.taskbarVisibility, [true, false]);
  window.setAppDetails = applyDetails;
  taskbar.apply(window);
  assert.equal(window.details.length, 2);
  assert.deepEqual(window.taskbarVisibility, [true, false, true, false]);
});

test("keeps updating the live icon if shortcut icon persistence fails", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false, () => {
    throw new Error("Shortcut is locked");
  });
  const window = fakeWindow();
  taskbar.select(fakeIcon(), "dark.ico");
  taskbar.apply(window);
  assert.equal(window.icons.length, 1);
  assert.equal(window.details[0]?.appIconPath, "dark.ico");
});

test("does not add a taskbar button for a window hidden to the tray", () => {
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false);
  const window = fakeWindow();
  taskbar.select(fakeIcon(), "light.ico");
  taskbar.apply(window);
  window.visible = false;
  taskbar.select(fakeIcon(), "dark.ico");
  taskbar.apply(window);
  assert.deepEqual(window.taskbarVisibility, []);
  assert.equal(window.details[1]?.appIconPath, "dark.ico");
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
