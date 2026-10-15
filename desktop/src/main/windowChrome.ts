import {
  ipcMain,
  nativeTheme,
  type BrowserWindow,
  type BrowserWindowConstructorOptions,
} from "electron";
import { DesktopIPC } from "../shared/desktopBridge.types";
import { isTrustedMainFrame } from "./navigationPolicy";

export const desktopTitlebarHeight = 48;
export function windowChromeOptions(
  platform: NodeJS.Platform,
  dark: boolean,
): BrowserWindowConstructorOptions {
  return {
    titleBarStyle: "hidden",
    backgroundColor: dark ? "#0c111d" : "#ffffff",
    ...(platform === "darwin"
      ? { trafficLightPosition: { x: 16, y: 18 } }
      : {
          titleBarOverlay: {
            height: desktopTitlebarHeight,
            color: dark ? "#0c111d" : "#ffffff",
            symbolColor: dark ? "#d0d5dd" : "#344054",
          },
          autoHideMenuBar: true,
        }),
  };
}
export function installWindowChrome(
  window: BrowserWindow,
  appOrigin: () => string,
): void {
  const state = () => ({
    focused: window.isFocused(),
    fullscreen: window.isFullScreen(),
  });
  const publish = () => {
    if (!window.isDestroyed() && !window.webContents.isDestroyed())
      window.webContents.send(DesktopIPC.windowState, state());
  };
  ipcMain.handle(DesktopIPC.getWindowState, (event) => {
    if (
      event.sender !== window.webContents ||
      event.senderFrame !== window.webContents.mainFrame ||
      !isTrustedMainFrame(event.sender, event.senderFrame.url, appOrigin())
    )
      throw new Error("Window chrome sender was rejected.");
    return state();
  });
  const updateTheme = () => {
    if (window.isDestroyed()) return;
    const options = windowChromeOptions(
      process.platform,
      nativeTheme.shouldUseDarkColors,
    );
    window.setBackgroundColor(options.backgroundColor!);
    if (process.platform !== "darwin")
      window.setTitleBarOverlay(
        options.titleBarOverlay as Electron.TitleBarOverlayOptions,
      );
  };
  nativeTheme.on("updated", updateTheme);
  window.on("focus", publish);
  window.on("blur", publish);
  window.on("enter-full-screen", publish);
  window.on("leave-full-screen", publish);
  window.once("closed", () => {
    nativeTheme.removeListener("updated", updateTheme);
    ipcMain.removeHandler(DesktopIPC.getWindowState);
  });
}
