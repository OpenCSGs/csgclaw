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
  assert.deepEqual(window.taskbarVisibility, []);
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

test("coalesces repeated theme events but reapplies the icon when shown again", async () => {
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
  assert.equal(syncs, 0);
  assert.equal(window.details[0]?.appIconPath, "light.ico");
  taskbar.apply(window, true);
  assert.equal(window.icons.length, 2);
  assert.deepEqual(window.taskbarVisibility, []);
  window.destroyed = true;
  taskbar.apply(window, true);
  assert.equal(window.icons.length, 2);
  window.destroyed = false;
  window.visible = false;
  await taskbar.refresh(window);
  assert.equal(syncs, 1);
  assert.equal(window.details.at(-1)?.appIconPath, "persistent-light.ico");
  await taskbar.refresh(window);
  assert.equal(syncs, 1);
});

test("rapid switches update live icons before persisting only the final shortcut", async () => {
  const writes: string[] = [];
  const window = fakeWindow();
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false, (path) => {
    assert.equal(window.icons.length, 3);
    writes.push(path);
    return `persistent-${path}`;
  }, async () => {});
  for (const path of ["light.ico", "dark.ico", "light.ico"]) {
    taskbar.select(fakeIcon(), path);
    taskbar.apply(window);
  }
  assert.deepEqual(writes, []);
  assert.deepEqual(window.taskbarVisibility, []);
  await taskbar.refresh(window);
  assert.deepEqual(writes, ["light.ico"]);
  assert.equal(window.details.at(-1)?.appIconPath, "persistent-light.ico");
});

test("refreshes the final taskbar icon after rapid theme changes settle", async () => {
  const taskbar = new WindowsTaskbarIcon(
    DesktopPlatform.Windows,
    false,
    (iconPath) => iconPath,
    async () => {},
  );
  const window = fakeWindow();
  const light = fakeIcon();
  const dark = fakeIcon();

  taskbar.select(light, "light.ico");
  taskbar.apply(window);
  taskbar.select(dark, "dark.ico");
  taskbar.apply(window);
  taskbar.select(light, "light.ico");
  taskbar.apply(window);
  await taskbar.refresh(window);

  assert.deepEqual(window.icons, [light, dark, light, light]);
  assert.equal(window.details.at(-1)?.appIconPath, "light.ico");
  assert.deepEqual(window.taskbarVisibility, [true, false]);
});

test("applies the latest selection while Explorer removes the old button", async () => {
  let finishRemoval: (() => void) | undefined;
  const taskbar = new WindowsTaskbarIcon(
    DesktopPlatform.Windows,
    false,
    (iconPath) => iconPath,
    () =>
      new Promise<void>((resolve) => {
        finishRemoval = resolve;
      }),
  );
  const window = fakeWindow();
  const light = fakeIcon();
  const dark = fakeIcon();

  taskbar.select(light, "light.ico");
  taskbar.apply(window);
  const refresh = taskbar.refresh(window);
  taskbar.select(dark, "dark.ico");
  taskbar.apply(window);
  finishRemoval?.();
  await refresh;

  assert.deepEqual(window.icons, [light, dark, dark]);
  assert.equal(window.details.at(-1)?.appIconPath, "dark.ico");
  assert.deepEqual(window.taskbarVisibility, [true, false]);
});

test("forces the latest icon after restoring the taskbar button", async () => {
  const operations: string[] = [];
  const taskbar = new WindowsTaskbarIcon(
    DesktopPlatform.Windows,
    false,
    (iconPath) => iconPath,
    async () => {},
    async () => {},
  );
  const window = fakeWindow();
  const setSkipTaskbar = window.setSkipTaskbar;
  const setIcon = window.setIcon;

  window.setSkipTaskbar = (skip: boolean) => {
    operations.push(skip ? "hide" : "show");
    setSkipTaskbar.call(window, skip);
  };
  window.setIcon = (image: NativeImage | string) => {
    operations.push("icon");
    setIcon.call(window, image);
  };

  const light = fakeIcon();
  const dark = fakeIcon();
  taskbar.select(light, "light.ico");
  taskbar.apply(window);
  taskbar.select(dark, "dark.ico");
  taskbar.apply(window);
  await taskbar.refresh(window);

  assert.deepEqual(operations, ["icon", "icon", "hide", "show", "icon"]);
  assert.equal(window.icons.at(-1), dark);
});

test("only the newest refresh restores the taskbar button", async () => {
  const finishRemovals: Array<() => void> = [];
  const taskbar = new WindowsTaskbarIcon(
    DesktopPlatform.Windows,
    false,
    (iconPath) => iconPath,
    () =>
      new Promise<void>((resolve) => {
        finishRemovals.push(resolve);
      }),
  );
  const window = fakeWindow();

  taskbar.select(fakeIcon(), "light.ico");
  taskbar.apply(window);
  const first = taskbar.refresh(window);
  const second = taskbar.refresh(window);

  finishRemovals[0]?.();
  await first;
  assert.deepEqual(window.taskbarVisibility, [true, true]);

  finishRemovals[1]?.();
  await second;
  assert.deepEqual(window.taskbarVisibility, [true, true, false]);
});

test("restores the taskbar button when the delayed icon update fails", async () => {
  const taskbar = new WindowsTaskbarIcon(
    DesktopPlatform.Windows,
    false,
    (iconPath) => iconPath,
    async () => {},
  );
  const window = fakeWindow();

  taskbar.select(fakeIcon(), "light.ico");
  taskbar.apply(window);
  window.setAppDetails = () => {
    throw new Error("Explorer unavailable");
  };

  await assert.rejects(taskbar.refresh(window), /Explorer unavailable/);
  assert.deepEqual(window.taskbarVisibility, [true, false]);
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
  assert.deepEqual(window.taskbarVisibility, []);
  window.setAppDetails = applyDetails;
  taskbar.apply(window);
  assert.equal(window.details.length, 2);
  assert.deepEqual(window.taskbarVisibility, []);
});

test("keeps updating the live icon if shortcut icon persistence fails", async () => {
  let attempts = 0;
  const taskbar = new WindowsTaskbarIcon(DesktopPlatform.Windows, false, () => {
    attempts++;
    throw new Error("Shortcut is locked");
  });
  const window = fakeWindow();
  window.visible = false;
  taskbar.select(fakeIcon(), "dark.ico");
  taskbar.apply(window);
  assert.equal(window.icons.length, 1);
  assert.equal(window.details[0]?.appIconPath, "dark.ico");
  await taskbar.refresh(window);
  await taskbar.refresh(window);
  assert.equal(attempts, 2);
  assert.equal(window.icons.length, 3);
  assert.equal(window.details.at(-1)?.appIconPath, "dark.ico");
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
