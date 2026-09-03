import { app, crashReporter, dialog } from "electron";
import electronSquirrelStartup from "electron-squirrel-startup";
import { AppLifecycle } from "./appLifecycle";
import {
  diagnosticLaunchFlags,
  initializeDesktopLogger,
  logDesktopError,
  logDesktopInfo,
} from "./desktopLogger";
import { isWindowsDesktop } from "./platform";
import { windowsAppUserModelID } from "./windowsTaskbar";

app.enableSandbox();
app.setName("CSGClaw");
if (!isWindowsDesktop || !process.windowsStore) {
  app.setAppUserModelId(
    isWindowsDesktop ? windowsAppUserModelID : "com.opencsg.csgclaw.desktop",
  );
}
app.setAppLogsPath();
const mainLogPath = initializeDesktopLogger(app.getPath("logs"));
logDesktopInfo("process-start", {
  arch: process.arch,
  crashDumpsPath: app.getPath("crashDumps"),
  flags: diagnosticLaunchFlags(process.argv),
  logPath: mainLogPath,
  packaged: app.isPackaged,
  platform: process.platform,
  version: app.getVersion(),
});
try {
  crashReporter.start({
    productName: "CSGClaw",
    uploadToServer: false,
    globalExtra: {
      arch: process.arch,
      platform: process.platform,
    },
  });
  logDesktopInfo("crash-reporter-started");
} catch (error) {
  logDesktopError("crash-reporter-failed", error);
}

process.on("uncaughtExceptionMonitor", (error, origin) => {
  logDesktopError("uncaught-exception", error, { origin });
});
process.on("warning", (warning) => {
  logDesktopError("process-warning", warning);
});
process.on("exit", (exitCode) => {
  logDesktopInfo("process-exit", { exitCode });
});
app.on("child-process-gone", (_event, details) => {
  logDesktopInfo("child-process-gone", {
    exitCode: details.exitCode,
    name: details.name,
    reason: details.reason,
    serviceName: details.serviceName,
    type: details.type,
  });
});
app.on("render-process-gone", (_event, _webContents, details) => {
  logDesktopInfo("render-process-gone", {
    exitCode: details.exitCode,
    reason: details.reason,
  });
});
app.on("before-quit", () => logDesktopInfo("before-quit"));
app.on("will-quit", () => logDesktopInfo("will-quit"));
app.on("quit", (_event, exitCode) => logDesktopInfo("quit", { exitCode }));

if (electronSquirrelStartup) {
  logDesktopInfo("squirrel-startup-exit", { flags: diagnosticLaunchFlags(process.argv) });
  app.quit();
} else {
  const hasSingleInstanceLock = app.requestSingleInstanceLock();
  if (!hasSingleInstanceLock) {
    logDesktopInfo("single-instance-lock-denied");
    app.quit();
  } else {
    logDesktopInfo("single-instance-lock-acquired");
    const lifecycle = new AppLifecycle();

    app.on("second-instance", (_event, argv) => {
      logDesktopInfo("second-instance", { flags: diagnosticLaunchFlags(argv) });
      lifecycle.handleSecondInstance();
    });
    app.on("activate", () => {
      logDesktopInfo("activate");
      lifecycle.show();
    });
    app.on("before-quit", (event) => lifecycle.handleBeforeQuit(event));
    app.on("window-all-closed", () => {
      logDesktopInfo("window-all-closed");
      // Closing the window keeps the sidecar and long-running agents alive.
    });

    void app
      .whenReady()
      .then(async () => {
        logDesktopInfo("electron-ready");
        await lifecycle.start();
      })
      .catch((error: unknown) => {
        logDesktopError("startup-failed", error);
        const message = error instanceof Error ? error.message : String(error);
        dialog.showErrorBox(
          "CSGClaw failed to start",
          `${message}\n\nMain log: ${mainLogPath}\nCrash dumps: ${app.getPath("crashDumps")}`,
        );
        logDesktopInfo("startup-exit", { exitCode: 1 });
        app.exit(1);
      });
  }
}
