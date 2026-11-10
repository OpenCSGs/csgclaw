import { describe, expect, it } from "vitest";
import {
  closeDialogSearch,
  openWorkspaceDialog,
  profileDialogSearch,
  settingsDialogSearch,
} from "@/models/workspaceDialogs";

describe("workspace dialog URLs", () => {
  it("preserves the underlying agent tab and task view while opening and closing Settings", () => {
    const opened = settingsDialogSearch("?tab=connectors&view=scheduled", "models");
    expect(new URLSearchParams(opened).get("dialog")).toBe("settings");
    expect(new URLSearchParams(opened).get("section")).toBe("models");
    expect(new URLSearchParams(closeDialogSearch(opened))).toEqual(
      new URLSearchParams("tab=connectors&view=scheduled"),
    );
  });
  it("cleans up the profile selection without changing the underlying query", () => {
    const opened = profileDialogSearch("?tab=skills", "u-admin");
    expect(openWorkspaceDialog(opened)).toBe("profile");
    expect(closeDialogSearch(opened)).toBe("tab=skills");
    expect(openWorkspaceDialog("?dialog=unknown")).toBe("");
  });
  it("cleans up the selected global connector when closing Settings", () => {
    expect(closeDialogSearch("?tab=apps&dialog=settings&section=connectors&app_resource=gitlab")).toBe("tab=apps");
  });
});
