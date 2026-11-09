import { getDesktopBridge, type DesktopOAuthPurpose } from "./desktopBridge";

export type ExternalNavigation = {
  close(): void;
  open(url: string): Promise<boolean>;
};

export function prepareOAuthNavigation(purpose: DesktopOAuthPurpose): ExternalNavigation {
  const desktop = getDesktopBridge();
  if (desktop) {
    return {
      close() {},
      async open(url: string) {
        return (await desktop.openOAuth({ purpose, url })).opened;
      },
    };
  }
  if (purpose === "opencsg-auth") {
    return {
      close() {},
      async open(url: string) {
        window.location.assign(url);
        return true;
      },
    };
  }

  const preparedWindow = openBlankWindow();
  return {
    close() {
      closeWindow(preparedWindow);
    },
    async open(url: string) {
      return navigateWindow(preparedWindow, url);
    },
  };
}

function openBlankWindow(): Window | null {
  try {
    return window.open("about:blank", "_blank");
  } catch {
    return null;
  }
}

function navigateWindow(targetWindow: Window | null, targetURL: string): boolean {
  if (targetWindow) {
    try {
      targetWindow.opener = null;
    } catch {
      // Some browser contexts make opener read-only.
    }
    try {
      targetWindow.location.href = targetURL;
      return true;
    } catch {
      closeWindow(targetWindow);
    }
  }
  try {
    const fallbackWindow = window.open(targetURL, "_blank");
    if (!fallbackWindow) {
      return false;
    }
    try {
      fallbackWindow.opener = null;
    } catch {
      // Some browser contexts make opener read-only.
    }
    return true;
  } catch {
    return false;
  }
}

function closeWindow(targetWindow: Window | null): void {
  if (!targetWindow) {
    return;
  }
  try {
    targetWindow.close();
  } catch {
    // The browser owns popup teardown.
  }
}
