import { app, dialog } from "electron";
import electronSquirrelStartup from "electron-squirrel-startup";
import { AppLifecycle } from "./appLifecycle";
import { isWindowsDesktop } from "./platform";

app.enableSandbox();
app.setName("CSGClaw");
if (!isWindowsDesktop || !process.windowsStore) {
  app.setAppUserModelId(
    isWindowsDesktop
      ? "com.squirrel.csgclaw_desktop.CSGClaw"
      : "com.opencsg.csgclaw.desktop",
  );
}
app.setAppLogsPath();

if (electronSquirrelStartup) {
  app.quit();
} else if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  const lifecycle = new AppLifecycle();

  app.on("second-instance", () => lifecycle.handleSecondInstance());
  app.on("activate", () => lifecycle.show());
  app.on("before-quit", (event) => lifecycle.handleBeforeQuit(event));
  app.on("window-all-closed", () => {
    // Closing the window keeps the sidecar and long-running agents alive.
  });

  void app
    .whenReady()
    .then(() => lifecycle.start())
    .catch((error: unknown) => {
      const message = error instanceof Error ? error.message : String(error);
      dialog.showErrorBox("CSGClaw failed to start", message);
      app.exit(1);
    });
}
