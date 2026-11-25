export const WORKSPACE_DIALOG_PARAM = "dialog";
export const SETTINGS_MARK_PARAM = "mark";

export const WorkspaceDialogNames = {
  settings: "settings",
  profile: "profile",
} as const;

export type WorkspaceDialogName = (typeof WorkspaceDialogNames)[keyof typeof WorkspaceDialogNames];

const DIALOG_OWNED_PARAMS: Record<WorkspaceDialogName, readonly string[]> = {
  settings: ["section", "app_resource", SETTINGS_MARK_PARAM],
  profile: ["human"],
};

function paramsOf(search: string): URLSearchParams {
  return new URLSearchParams(search || "");
}

export function openWorkspaceDialog(search: string): WorkspaceDialogName | "" {
  const value = paramsOf(search).get(WORKSPACE_DIALOG_PARAM) || "";
  return value === WorkspaceDialogNames.settings || value === WorkspaceDialogNames.profile ? value : "";
}

export function settingsDialogSearch(search: string, section = "", mark?: "version"): string {
  const next = paramsOf(search);
  next.set(WORKSPACE_DIALOG_PARAM, WorkspaceDialogNames.settings);
  if (section) {
    next.set("section", section);
  }
  if (mark && mark === section) {
    next.set(SETTINGS_MARK_PARAM, mark);
  } else {
    next.delete(SETTINGS_MARK_PARAM);
  }
  return next.toString();
}

export function profileDialogSearch(search: string, humanID: string): string {
  const next = paramsOf(search);
  next.set(WORKSPACE_DIALOG_PARAM, WorkspaceDialogNames.profile);
  next.set("human", String(humanID || ""));
  return next.toString();
}

export function closeDialogSearch(search: string): string {
  const next = paramsOf(search);
  next.delete(WORKSPACE_DIALOG_PARAM);
  for (const keys of Object.values(DIALOG_OWNED_PARAMS)) {
    for (const key of keys) {
      next.delete(key);
    }
  }
  return next.toString();
}

export function dialogParam(search: string, key: "section" | "human"): string {
  return paramsOf(search).get(key) || "";
}
