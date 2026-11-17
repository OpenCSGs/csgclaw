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

export function isDesktopReloadShortcut(
  input: Electron.Input,
  platform: NodeJS.Platform,
): boolean {
  if (input.type !== "keyDown" || input.alt || input.shift) return false;
  if (input.key.toLowerCase() === "r")
    return platform === "darwin"
      ? input.meta && !input.control
      : input.control && !input.meta;
  return platform !== "darwin" && input.key === "F5" && !input.control && !input.meta;
}
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
  // BrowserWindow getters are no longer usable when the "closed" event fires.
  const webContents = window.webContents;
  let appearance: DesktopWindowChromeAppearance = "default";
  const state = () => ({
    focused: window.isFocused(),
    fullscreen: window.isFullScreen(),
  });
  const publish = () => {
    if (!window.isDestroyed() && !webContents.isDestroyed())
      webContents.send(DesktopIPC.windowState, state());
  };
  const assertSender = (event: IpcMainInvokeEvent) => {
    if (
      event.sender !== webContents ||
      event.senderFrame !== webContents.mainFrame ||
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
  webContents.on("did-start-navigation", resetAppearance);
  const reload = (event: Electron.Event, input: Electron.Input) => {
    if (!isDesktopReloadShortcut(input, process.platform)) return;
    event.preventDefault();
    if (!input.isAutoRepeat) webContents.reload();
  };
  webContents.on("before-input-event", reload);
  // A replaced main document must regain renderer focus after a native Reload.
  // Do not activate a background window when a navigation finishes.
  const restoreRendererFocus = () => {
    if (!window.isDestroyed() && window.isFocused()) webContents.focus();
  };
  webContents.on("did-finish-load", restoreRendererFocus);
  nativeTheme.on("updated", updateTheme);
  window.on("focus", publish);
  window.on("blur", publish);
  window.on("enter-full-screen", publish);
  window.on("leave-full-screen", publish);
  window.once("closed", () => {
    nativeTheme.removeListener("updated", updateTheme);
    ipcMain.removeHandler(DesktopIPC.getWindowState);
    ipcMain.removeHandler(DesktopIPC.setWindowChromeAppearance);
    webContents.removeListener("did-start-navigation", resetAppearance);
    webContents.removeListener("before-input-event", reload);
    webContents.removeListener("did-finish-load", restoreRendererFocus);
  });
}
