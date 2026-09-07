import fs from "node:fs";
import path from "node:path";
import type { DesktopThemeSource } from "../shared/desktopBridge.types";
import { parseDesktopThemeSource } from "../shared/desktopTheme";

const desktopThemeSourceFileName = "desktop-theme-source.json";
const defaultDesktopThemeSource: DesktopThemeSource = "dark";

export function readDesktopThemeSourcePreference(
  userData: string,
  onError: (error: unknown) => void = () => {},
): DesktopThemeSource {
  try {
    const raw = fs.readFileSync(
      path.join(userData, desktopThemeSourceFileName),
      "utf8",
    );
    const parsed = JSON.parse(raw) as { theme?: unknown };
    return parseDesktopThemeSource(parsed.theme);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
      onError(error);
    }
    return defaultDesktopThemeSource;
  }
}

export function writeDesktopThemeSourcePreference(
  userData: string,
  theme: DesktopThemeSource,
): void {
  fs.mkdirSync(userData, { recursive: true });
  fs.writeFileSync(
    path.join(userData, desktopThemeSourceFileName),
    `${JSON.stringify({ theme })}\n`,
    { encoding: "utf8", mode: 0o600 },
  );
}
