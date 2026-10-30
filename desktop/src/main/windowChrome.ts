import {
  ipcMain,
  nativeTheme,
  type BrowserWindow,
  type BrowserWindowConstructorOptions,
  type IpcMainInvokeEvent,
} from "electron";
import {
  DesktopIPC,
  type DesktopWindowChromeAppearance,
} from "../shared/desktopBridge.types";
import { isTrustedMainFrame } from "./navigationPolicy";

export const desktopTitlebarHeight = 48;
export function windowChromeOptions(
  platform: NodeJS.Platform,
  dark: boolean,
  appearance: DesktopWindowChromeAppearance = "default",
): BrowserWindowConstructorOptions {
  const backgroundColor =
    appearance === "onboarding" ? "#f4f5ee" : dark ? "#0c111d" : "#ffffff";
  return {
    titleBarStyle: "hidden",
    backgroundColor,
    ...(platform === "darwin"
      ? { trafficLightPosition: { x: 16, y: 18 } }
      : {
          titleBarOverlay: {
            height: desktopTitlebarHeight,
            color: backgroundColor,
            symbolColor:
              appearance === "default" && dark ? "#d0d5dd" : "#344054",
          },
          autoHideMenuBar: true,
        }),
  };
}
export function installWindowChrome(
  window: BrowserWindow,
  appOrigin: () => string,
): void {
  let appearance: DesktopWindowChromeAppearance = "default";
  const state = () => ({
    focused: window.isFocused(),
    fullscreen: window.isFullScreen(),
  });
  const publish = () => {
    if (!window.isDestroyed() && !window.webContents.isDestroyed())
      window.webContents.send(DesktopIPC.windowState, state());
  };
  const assertSender = (event: IpcMainInvokeEvent) => {
    if (
      event.sender !== window.webContents ||
      event.senderFrame !== window.webContents.mainFrame ||
      !isTrustedMainFrame(event.sender, event.senderFrame.url, appOrigin())
    )
      throw new Error("Window chrome sender was rejected.");
  };
  ipcMain.handle(DesktopIPC.getWindowState, (event) => {
    assertSender(event);
    return state();
  });
  const updateTheme = () => {
    if (window.isDestroyed()) return;
    const options = windowChromeOptions(
      process.platform,
      nativeTheme.shouldUseDarkColors,
      appearance,
    );
    window.setBackgroundColor(options.backgroundColor!);
    if (process.platform !== "darwin")
      window.setTitleBarOverlay(
        options.titleBarOverlay as Electron.TitleBarOverlayOptions,
      );
  };
  ipcMain.handle(
    DesktopIPC.setWindowChromeAppearance,
    (event, input: unknown) => {
      assertSender(event);
      if (input !== "default" && input !== "onboarding") {
        throw new Error("Window chrome appearance was rejected.");
      }
      // Change only this window's controls, never nativeTheme or saved preferences.
      appearance = input;
      updateTheme();
    },
  );
  const resetAppearance = (
    _event: Electron.Event,
    _url: string,
    isInPlace: boolean,
    isMainFrame: boolean,
  ) => {
    if (!isMainFrame || isInPlace) return;
    appearance = "default";
    updateTheme();
  };
  window.webContents.on("did-start-navigation", resetAppearance);
  nativeTheme.on("updated", updateTheme);
  window.on("focus", publish);
  window.on("blur", publish);
  window.on("enter-full-screen", publish);
  window.on("leave-full-screen", publish);
  window.once("closed", () => {
    nativeTheme.removeListener("updated", updateTheme);
    ipcMain.removeHandler(DesktopIPC.getWindowState);
    ipcMain.removeHandler(DesktopIPC.setWindowChromeAppearance);
    window.webContents.removeListener("did-start-navigation", resetAppearance);
  });
}
