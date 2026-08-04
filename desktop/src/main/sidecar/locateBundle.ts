import fs from "node:fs";
import path from "node:path";
import { app } from "electron";
import {
  csgclawExecutableForPlatform,
  goArchForDesktopArch,
  goOSForDesktopPlatform,
} from "../../shared/desktopEnvironment";

export type BackendBundle = {
  executable: string;
  root: string;
};

export function locateBackendBundle(): BackendBundle {
  const binaryName = csgclawExecutableForPlatform(process.platform);
  const goOS = goOSForDesktopPlatform(process.platform);
  const goArch = goArchForDesktopArch(process.arch);
  const configured = process.env.CSGCLAW_DESKTOP_BACKEND?.trim();
  const candidates = configured
    ? [configured]
    : app.isPackaged
      ? [path.join(process.resourcesPath, "backend", "csgclaw", "bin", binaryName)]
      : [
          path.resolve(app.getAppPath(), "..", "bin", binaryName),
          path.resolve(
            app.getAppPath(),
            "out",
            "input",
            `${goOS}-${goArch}`,
            "backend",
            "csgclaw",
            "bin",
            binaryName,
          ),
        ];

  for (const candidate of candidates) {
    if (!candidate || !fs.existsSync(candidate)) {
      continue;
    }
    const executable = fs.realpathSync(candidate);
    return {
      executable,
      root: path.dirname(path.dirname(executable)),
    };
  }
  throw new Error(`CSGClaw backend executable was not found. Checked: ${candidates.join(", ")}`);
}
