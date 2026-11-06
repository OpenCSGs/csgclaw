import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { fetchCloudAgentTemplate } from "@/api/relay";
import { createQueryWrapper } from "../../../../../tests/helpers/queryClient";
import { cloudAgentFixture } from "../../../../../tests/helpers/cloudAgentFixture";
import { AgentPlaza, AgentPlazaMetadata, CloudAgentDetail } from "./AgentPlaza";

vi.mock("@/api/relay", () => ({ fetchCloudAgentTemplate: vi.fn() }));

it("passes the authorized template repository to the shared profile renderer", async () => {
  vi.mocked(fetchCloudAgentTemplate).mockResolvedValue({ repo_path: "alice/writing-template" });
  const agent = { ...cloudAgentFixture[0], publisher: "Agentichub", modelID: "instance-model" };
  const renderTemplate = vi.fn((repoPath: string) => <h3>{repoPath}</h3>);
  render(
    <CloudAgentDetail
      agent={agent}
      t={createTranslator("zh")}
      onNewTask={vi.fn()}
      onRemove={vi.fn()}
      renderTemplate={renderTemplate}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  expect(await screen.findByRole("heading", { name: "alice/writing-template" })).toBeVisible();
  expect(renderTemplate).toHaveBeenCalledWith("alice/writing-template", expect.anything());
  expect(fetchCloudAgentTemplate).toHaveBeenCalledWith(agent, expect.any(AbortSignal));
});

it("uses the catalog template reference without requesting it again", async () => {
  vi.mocked(fetchCloudAgentTemplate).mockClear();
  const agent = { ...cloudAgentFixture[0], publisher: "Agentichub", templateRepoPath: "alice/worker" };
  render(
    <CloudAgentDetail
      agent={agent}
      t={createTranslator("zh")}
      onNewTask={vi.fn()}
      onRemove={vi.fn()}
      renderTemplate={(path) => <h3>{path}</h3>}
    />,
    { wrapper: createQueryWrapper().wrapper },
  );
  expect(screen.getByRole("heading", { name: "alice/worker" })).toBeVisible();
  expect(fetchCloudAgentTemplate).not.toHaveBeenCalled();
});

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

it("shows the creator and product publisher as metadata", () => {
  const agent = { ...cloudAgentFixture[0], creatorName: "Alice", publisher: "CSGClaw" };
  render(<AgentPlazaMetadata agent={agent} t={createTranslator("zh")} />);
  expect(screen.getByText("Alice")).toBeVisible();
  expect(screen.getByText("CSGClaw")).toBeVisible();
  expect(screen.getAllByRole("term")).toHaveLength(2);
  for (const value of [agent.name, agent.id, agent.relayURL, agent.sourceAgentID])
    expect(screen.queryByText(value)).not.toBeInTheDocument();
});

it("hides the source row when the creator is unavailable", () => {
  render(<AgentPlazaMetadata agent={cloudAgentFixture[0]} t={createTranslator("zh")} />);
  expect(screen.queryByText("来源")).not.toBeInTheDocument();
  expect(screen.getByText("发布端")).toBeVisible();
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
