import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { cloudAgentFixture } from "../../../../../tests/helpers/cloudAgentFixture";
import { AgentPlaza, AgentPlazaMetadata } from "./AgentPlaza";

describe("cloud directory availability", () => {
  it.each([
    ["online", "在线"],
    ["offline", "离线"],
  ] as const)("shows only the %s status and allows adding an offline listing", (status, label) => {
    render(
      <AgentPlaza
        agents={[{ ...cloudAgentFixture[0], status }]}
        addedIDs={[]}
        t={createTranslator("zh")}
        onAdd={vi.fn()}
        onOpen={vi.fn()}
      />,
    );
    expect(screen.getByText(label)).toBeVisible();
    expect(screen.queryByText(status === "online" ? "离线" : "在线")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "添加到我的智能体" })).toBeEnabled();
  });
});

it("shows only the agent display name and product publisher as metadata", () => {
  const agent = { ...cloudAgentFixture[0], publisher: "CSGClaw" };
  render(<AgentPlazaMetadata agent={agent} t={createTranslator("zh")} />);
  expect(screen.getByText(agent.name)).toBeVisible();
  expect(screen.getByText("CSGClaw")).toBeVisible();
  expect(screen.getAllByRole("term")).toHaveLength(2);
  for (const value of [agent.id, agent.relayURL, agent.sourceAgentID])
    expect(screen.queryByText(value)).not.toBeInTheDocument();
});

it("shows a private agent under Mine before it has been added", async () => {
  const agent = { ...cloudAgentFixture[0], visibility: "private" } as const;
  const onAdd = vi.fn();
  const user = userEvent.setup();
  render(<AgentPlaza agents={[agent]} addedIDs={[]} t={createTranslator("zh")} onAdd={onAdd} onOpen={vi.fn()} />);
  expect(screen.queryByRole("heading", { name: agent.name })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "我的" }));
  expect(screen.getByRole("heading", { name: agent.name })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "添加到我的智能体" }));
  expect(onAdd).toHaveBeenCalledWith(agent.id);
});
