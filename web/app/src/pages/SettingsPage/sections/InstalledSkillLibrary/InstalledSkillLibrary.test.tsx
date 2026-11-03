import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { acknowledgeDiscoveredSkills, rescanDiscoveredSkills } from "@/api/skills";
import { createTranslator } from "@/shared/i18n";
import type { SkillSummary } from "@/models/skillhub";
import { InstalledSkillLibrary } from "./InstalledSkillLibrary";
vi.mock("@/api/skills", () => ({ acknowledgeDiscoveredSkills: vi.fn(), rescanDiscoveredSkills: vi.fn() }));
const source = { tool: "Codex", path: "/test/.codex/skills/pdf", updated_at: "2026-10-30" };
const skills: SkillSummary[] = [
  {
    name: "pdf",
    description: "Read documents",
    discovery: { ...source, key: "pdf", new: true, sources: [source, { ...source, tool: "Claude Code" }] },
  },
  { name: "local", source: "local" },
  { name: "builtin", source: "system" },
  { name: "market", source: "market" },
];
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(rescanDiscoveredSkills).mockResolvedValue(skills);
  vi.mocked(acknowledgeDiscoveredSkills).mockResolvedValue();
});
describe("compact installed skill library", () => {
  it("renders a flat card list and acknowledges a new skill when its details open", async () => {
    const open = vi.fn();
    const refresh = vi.fn().mockResolvedValue({ isError: false });
    render(
      <InstalledSkillLibrary skills={skills} query="" onOpen={open} onRefresh={refresh} t={createTranslator("zh")} />,
    );
    expect(screen.getAllByRole("article")).toHaveLength(4);
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "收起全部" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /^pdf/ }));
    expect(open).toHaveBeenCalledWith(skills[0]);
    await waitFor(() => expect(acknowledgeDiscoveredSkills).toHaveBeenCalledWith(["pdf"]));
    expect(refresh).toHaveBeenCalledOnce();
  });
  it("filters the same deduplicated skill by its secondary app and includes other source types", async () => {
    const user = userEvent.setup();
    render(
      <InstalledSkillLibrary
        skills={skills}
        query=""
        onOpen={vi.fn()}
        onRefresh={vi.fn()}
        t={createTranslator("zh")}
      />,
    );
    await user.click(screen.getByRole("combobox", { name: "筛选技能来源" }));
    for (const name of ["Claude Code (1)", "Codex (1)", "技能市场 (1)", "本地安装 (1)", "内建 (1)"])
      expect(screen.getByRole("option", { name })).toBeVisible();
    await user.click(screen.getByRole("option", { name: "Claude Code (1)" }));
    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(screen.getByRole("button", { name: /^pdf/ })).toBeVisible();
    expect(screen.getByText("1 / 4 项技能")).toBeVisible();
  });
  it("keeps rescanning available for an empty library and refreshes restored links", async () => {
    const refresh = vi.fn().mockResolvedValue({ isError: false });
    render(
      <InstalledSkillLibrary skills={[]} query="" onOpen={vi.fn()} onRefresh={refresh} t={createTranslator("zh")} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "重新扫描" }));
    expect(await screen.findByRole("status")).toHaveTextContent("已关联 1 项本机技能");
    expect(rescanDiscoveredSkills).toHaveBeenCalledOnce();
    expect(refresh).toHaveBeenCalledOnce();
  });
  it("reports scan failure and enables retry without claiming success", async () => {
    vi.mocked(rescanDiscoveredSkills).mockRejectedValue(new Error("offline"));
    const refresh = vi.fn();
    render(
      <InstalledSkillLibrary
        skills={skills}
        query=""
        onOpen={vi.fn()}
        onRefresh={refresh}
        t={createTranslator("zh")}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "重新扫描" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("失败");
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(refresh).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "重新扫描" })).toBeEnabled();
  });
  it("replaces the ambiguous plus-count with an explicit version and software count", async () => {
    const item: SkillSummary = { ...skills[0], discovery: { ...skills[0].discovery!, version: "0.9.1" } };
    const open = vi.fn();
    render(
      <InstalledSkillLibrary skills={[item]} query="" onOpen={open} onRefresh={vi.fn()} t={createTranslator("zh")} />,
    );
    expect(screen.getByText("v0.9.1 · 2 个软件")).toBeVisible();
    expect(screen.queryByText("Codex +1")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "查看 pdf 的版本与来源" }));
    expect(open).toHaveBeenCalledWith(item);
  });
});
