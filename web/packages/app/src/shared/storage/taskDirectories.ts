import { TASK_DIRECTORIES_STORAGE_KEY } from "./keys";
import { writeWorkspacePreference } from "./workspacePreferences";

export function readTaskDirectories(): string[] {
  try {
    const paths: unknown = JSON.parse(window.localStorage.getItem(TASK_DIRECTORIES_STORAGE_KEY) || "[]");
    return Array.isArray(paths)
      ? [...new Set(paths.filter((path): path is string => typeof path === "string" && Boolean(path.trim())))].slice(
          0,
          20,
        )
      : [];
  } catch {
    return [];
  }
}

export function rememberTaskDirectory(path: string): string[] {
  const paths = [...new Set([path, ...readTaskDirectories()].filter(Boolean))].slice(0, 20);
  writeWorkspacePreference(TASK_DIRECTORIES_STORAGE_KEY, JSON.stringify(paths));
  return paths;
}
