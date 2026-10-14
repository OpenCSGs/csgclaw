import { detectInitialLocale } from "@/shared/i18n";
import { readCollapsedWorkspaceGroups, readSidebarCollapsed, writeWorkspacePreference } from "./workspacePreferences";
import { SIDEBAR_COLLAPSED_STORAGE_KEY } from "./keys";

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

test("restricted storage preserves usable application defaults and in-memory preference updates", () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("Restricted storage");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("Restricted storage");
  });
  expect(readSidebarCollapsed()).toBe(false);
  expect(readCollapsedWorkspaceGroups()).toEqual({ "direct-messages": false, rooms: true, threads: true });
  expect(detectInitialLocale()).toBe(navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en");
  expect(() => writeWorkspacePreference(SIDEBAR_COLLAPSED_STORAGE_KEY, "true")).not.toThrow();
});
