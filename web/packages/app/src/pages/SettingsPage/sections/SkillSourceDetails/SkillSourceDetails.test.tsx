import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { createTranslator } from "@/shared/i18n";
import type { SkillSummary } from "@/models/skillhub";
import { SkillSourceDetails } from "./SkillSourceDetails";
const selected = { tool: "Agents", path: "/shared/baocut", version: "0.9.1", updated_at: "2026-07-27" };
const skill: SkillSummary = {
  name: "baocut",
  discovery: {
    ...selected,
    key: "baocut",
    sources: [selected, { ...selected, tool: "Claude Code" }, { ...selected, tool: "Codex" }],
  },
};
describe("SkillSourceDetails", () => {
  it("explains that several software sources share one physical skill rather than competing versions", () => {
    render(<SkillSourceDetails skill={skill} locale="zh" t={createTranslator("zh")} />);
    expect(screen.getByRole("region", { name: "技能来源与版本" })).toHaveTextContent(
      "Agents、Claude Code、Codex 共用同一目录，实际是同一份技能",
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
  it("identifies the selected directory/version and exposes other versions with the selection rule", async () => {
    const duplicate: SkillSummary = {
      ...skill,
      discovery: {
        ...skill.discovery!,
        sources: [
          ...skill.discovery!.sources,
          { tool: "Pi", path: "/other/baocut", version: "0.8.0", updated_at: "2026-10-30" },
        ],
      },
    };
    render(<SkillSourceDetails skill={duplicate} locale="zh" t={createTranslator("zh")} />);
    await userEvent.click(screen.getByRole("button", { name: "查看 2 份来源与版本" }));
    expect(screen.getByText(/有效版本号优先/)).toBeVisible();
    const active = screen.getByText("正在使用").closest("li");
    expect(active).toHaveTextContent("v0.9.1");
    expect(active).toHaveTextContent("/shared/baocut");
    expect(screen.getByText("v0.8.0")).toBeVisible();
    expect(screen.getByText("/other/baocut")).toBeVisible();
  });
  it("labels missing versions honestly", async () => {
    const unversioned: SkillSummary = {
      ...skill,
      discovery: {
        ...skill.discovery!,
        version: undefined,
        sources: [
          { ...selected, version: undefined },
          { ...selected, path: "/other/baocut", version: undefined },
        ],
      },
    };
    render(<SkillSourceDetails skill={unversioned} locale="en" t={createTranslator("en")} />);
    await userEvent.click(screen.getByRole("button", { name: "Compare 2 source versions" }));
    expect(screen.getAllByText("Version not declared")).toHaveLength(2);
  });
});
