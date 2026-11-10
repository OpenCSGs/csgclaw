import { pickHostDirectoryRequest } from "@/api/agents";

type DirectoryHandleWithPath = {
  path?: string;
};

type DirectoryPickerWindow = Window & {
  csgclawDesktop?: unknown;
  showDirectoryPicker?: () => Promise<DirectoryHandleWithPath>;
};

type DirectoryInput = HTMLInputElement & {
  webkitdirectory?: boolean;
  directory?: boolean;
};

type DirectoryPickerFile = File & {
  path?: string;
};

function normalizeDirectoryPath(value: string | null | undefined): string | null {
  const path = String(value ?? "").trim();
  return path ? path : null;
}

function dirnameFromFilePath(filePath: string | null | undefined): string | null {
  const path = normalizeDirectoryPath(filePath);
  if (!path) {
    return null;
  }
  const trimmed = path.replace(/[\\/]+$/, "");
  const lastSeparator = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  if (lastSeparator <= 0) {
    return null;
  }
  return trimmed.slice(0, lastSeparator);
}

function extractDirectoryPath(files: FileList | null): string | null {
  if (!files || files.length === 0) {
    return null;
  }
  const firstFile = files.item(0) as DirectoryPickerFile | null;
  return dirnameFromFilePath(firstFile?.path);
}

function pickDirectoryPathFromInput(): Promise<string | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input") as DirectoryInput;
    input.type = "file";
    input.multiple = true;
    input.setAttribute("webkitdirectory", "");
    input.setAttribute("directory", "");
    input.webkitdirectory = true;
    input.directory = true;
    input.style.position = "fixed";
    input.style.opacity = "0";
    input.style.pointerEvents = "none";

    const finalize = (path: string | null) => {
      input.remove();
      resolve(path);
    };

    input.addEventListener(
      "change",
      () => {
        finalize(extractDirectoryPath(input.files));
      },
      { once: true },
    );
    input.addEventListener(
      "cancel",
      () => {
        finalize(null);
      },
      { once: true },
    );

    document.body.appendChild(input);
    input.click();
  });
}

export function rendererDirectoryPickerAvailable(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return Boolean(window.csgclawDesktop);
}

export async function pickLocalDirectoryPath(): Promise<string | null> {
  const picked = await pickHostDirectoryRequest();
  if (picked.status === "selected") {
    return picked.path;
  }
  if (picked.status === "canceled") {
    return null;
  }
  if (picked.status !== "unavailable") {
    return null;
  }
  const hostWindow = window as DirectoryPickerWindow;
  if (typeof hostWindow.showDirectoryPicker === "function") {
    try {
      const handle = await hostWindow.showDirectoryPicker();
      const directPath = normalizeDirectoryPath(handle?.path);
      if (directPath) {
        return directPath;
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return null;
      }
      throw error;
    }
  }
  return pickDirectoryPathFromInput();
}
