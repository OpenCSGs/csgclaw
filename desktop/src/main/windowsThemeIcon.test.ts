import assert from "node:assert/strict";
import test from "node:test";
import { windowsThemeIconName, WindowsTaskbarRefreshScheduler } from "./windowsThemeIcon";

test("continuous clicks refresh at fixed deadlines using the latest selection", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const scheduler = new WindowsTaskbarRefreshScheduler();
  let selected = "light";
  const shown: string[] = [];
  const refresh = () => shown.push(selected);
  scheduler.request(refresh);
  for (let i = 0; i < 9; i++) {
    t.mock.timers.tick(10);
    selected = i % 2 ? "light" : "dark";
    scheduler.request(refresh);
  }
  assert.deepEqual(shown, []);
  t.mock.timers.tick(10);
  assert.deepEqual(shown, ["dark"]);
  selected = "light";
  scheduler.request(refresh);
  t.mock.timers.tick(100);
  assert.deepEqual(shown, ["dark", "light"]);
  t.mock.timers.tick(1000);
  assert.equal(shown.length, 2);
});

test("cleanup cancels a pending taskbar refresh", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const scheduler = new WindowsTaskbarRefreshScheduler();
  let calls = 0;
  scheduler.request(() => calls++);
  scheduler.cancel();
  t.mock.timers.tick(100);
  assert.equal(calls, 0);
  scheduler.request(() => calls++);
  t.mock.timers.tick(100);
  assert.equal(calls, 1);
});

test("uses the app theme before the system theme for Windows icons", () => {
  assert.equal(
    windowsThemeIconName("light", true),
    "csgclaw-taskbar-light.ico",
  );
  assert.equal(
    windowsThemeIconName("light", false),
    "csgclaw-taskbar-light.ico",
  );
  assert.equal(
    windowsThemeIconName("dark", true),
    "csgclaw-taskbar-dark.ico",
  );
  assert.equal(
    windowsThemeIconName("dark", false),
    "csgclaw-taskbar-dark.ico",
  );
});

test("follows the system theme for Windows icons only in system mode", () => {
  assert.equal(
    windowsThemeIconName("system", false),
    "csgclaw-taskbar-light.ico",
  );
  assert.equal(
    windowsThemeIconName("system", true),
    "csgclaw-taskbar-dark.ico",
  );
});
