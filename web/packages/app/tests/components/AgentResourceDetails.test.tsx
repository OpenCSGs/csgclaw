import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fetchAgentSkills, fetchAgentSkillsFile } from "@/api/agents";
import { probeMCPServerRequest } from "@/api/mcp";
import { AgentResourceDetails } from "@/components/business/AgentDetails/AgentResourceDetails/AgentResourceDetails";
import type { MCPProbeResult, MCPServer } from "@/models/mcp";
import { createTranslator } from "@/shared/i18n";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/agents", () => ({ fetchAgentSkills: vi.fn(), fetchAgentSkillsFile: vi.fn() }));
vi.mock("@/api/mcp", () => ({ probeMCPServerRequest: vi.fn() }));

const t = createTranslator("zh");
const props = {
  agentID: "worker",
  t,
  busy: false,
  canToggle: true,
  onClose: vi.fn(),
  onToggle: vi.fn(),
  onDelete: vi.fn(),
};
const server: MCPServer = {
  name: "mcp_knowledge",
  description: "Engineering handbook",
  config: {
    name: "Engineering",
    display_name: "Engineering",
    description: "Engineering handbook",
    source: "agentichub",
    enabled: false,
    url: "https://user:password@example.test/mcp?token=secret#private",
    headers: { Authorization: "Bearer secret" },
    _meta: {
      "com.opencsg/mcp": {
        type: "llm_wiki",
        resource_id: "42",
        content_id: "content-42",
        auth_type: "csghub_access_token",
      },
    },
  },
};
const result: MCPProbeResult = {
  connected: true,
  durationMs: 12,
  tools: [{ name: "search" }],
  toolsSupported: true,
  truncated: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(fetchAgentSkills).mockResolvedValue({ entries: [] });
  vi.mocked(fetchAgentSkillsFile).mockResolvedValue({ path: "alpha/SKILL.md", content: "# Alpha" });
});

it.each([
  ["builtin", "内建"],
  ["market", "技能市场"],
  ["local", "本地上传"],
  ["unknown", "来源未知"],
  [undefined, "来源未知"],
] as const)("shows the recorded %s skill origin and the agent removal action", async (source, badge) => {
  const user = userEvent.setup();
  render(<AgentResourceDetails {...props} skill={{ name: "alpha", source }} />, {
    wrapper: createQueryWrapper().wrapper,
  });

  expect(screen.getByText(badge)).toBeInTheDocument();
  if (source === "unknown" || source === undefined)
    expect(screen.getByText(badge)).toHaveAttribute("title", t("iaSkillsBadgeUntrackedHint"));
  expect(screen.queryByRole("button", { name: "测试连接" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "从本智能体移除" }));
  expect(props.onDelete).toHaveBeenCalledOnce();
});

it("shows the knowledge base document and masks credentials", () => {
  render(<AgentResourceDetails {...props} server={server} />, { wrapper: createQueryWrapper().wrapper });

  expect(screen.getByRole("heading", { name: "Engineering" })).toBeInTheDocument();
  expect(screen.getByText("AgenticHub")).toBeInTheDocument();
  const document = JSON.parse(screen.getByLabelText(t("iaMCPConfigTitle")).textContent || "{}");
  expect(document).toEqual({
    mcpServers: {
      mcp_knowledge: {
        ...server.config,
        url: "https://example.test/mcp",
        headers: { Authorization: "••••••" },
      },
    },
  });
  expect(screen.getByRole("switch")).not.toBeChecked();
});

it("tests the saved knowledge configuration and reports the result", async () => {
  const user = userEvent.setup();
  let complete!: (value: MCPProbeResult) => void;
  vi.mocked(probeMCPServerRequest).mockReturnValue(new Promise((resolve) => (complete = resolve)));
  render(<AgentResourceDetails {...props} server={server} />, { wrapper: createQueryWrapper().wrapper });

  await user.click(screen.getByRole("button", { name: "测试连接" }));
  expect(probeMCPServerRequest).toHaveBeenCalledWith({ name: server.name, config: server.config }, expect.anything());
  expect(screen.getByRole("button", { name: "测试中..." })).toBeDisabled();
  await act(async () => complete(result));
  await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("连接成功"));
  expect(screen.getByRole("status")).toHaveTextContent("1 个工具");
});

it("allows retrying a failed knowledge connection test", async () => {
  const user = userEvent.setup();
  vi.mocked(probeMCPServerRequest).mockRejectedValueOnce(new Error("Unavailable")).mockResolvedValueOnce(result);
  render(<AgentResourceDetails {...props} server={server} />, { wrapper: createQueryWrapper().wrapper });

  await user.click(screen.getByRole("button", { name: "测试连接" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(t("resourcesMCPTestFailed"));
  await user.click(screen.getByRole("button", { name: "测试连接" }));
  await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
  await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("连接成功"));
});

it.each([true, false])("ignores the previous probe result after reopening (success: %s)", async (success) => {
  const user = userEvent.setup();
  let complete!: (value: MCPProbeResult) => void;
  let fail!: (error: Error) => void;
  vi.mocked(probeMCPServerRequest).mockReturnValue(
    new Promise((resolve, reject) => {
      complete = resolve;
      fail = reject;
    }),
  );
  const { wrapper } = createQueryWrapper();
  const first = render(<AgentResourceDetails {...props} server={server} />, { wrapper });
  await user.click(screen.getByRole("button", { name: "测试连接" }));
  first.unmount();
  render(<AgentResourceDetails {...props} server={server} />, { wrapper });

  await act(async () => {
    if (success) complete(result);
    else fail(new Error("Unavailable"));
  });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "测试连接" })).toBeEnabled();
});
