import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchCloudAgentResponse, sendCloudAgentMessage } from "@/api/relay";
import { cloudAgentFixture } from "../../../../../tests/helpers/cloudAgentFixture";
import { CLOUD_CHAT_STORAGE_KEY } from "@/shared/storage/keys";
import { createTranslator } from "@/shared/i18n";
import { CloudAgentChat } from "./CloudAgentChat";
vi.mock("@/api/relay", () => ({ sendCloudAgentMessage: vi.fn(), fetchCloudAgentResponse: vi.fn() }));
const agent = cloudAgentFixture[0];
const key = `${CLOUD_CHAT_STORAGE_KEY}:${agent.connectionID}:${agent.id}`;
const completed = (id: string, text: string) => ({
  id,
  model: agent.id,
  status: "completed" as const,
  output: [{ content: [{ type: "output_text", text }] }],
});
beforeEach(() => {
  localStorage.removeItem(key);
  vi.resetAllMocks();
  vi.mocked(sendCloudAgentMessage).mockResolvedValue(completed("resp_first", "hello back"));
});
describe("cloud agent chat", () => {
  it("sends real target references, continues the response chain and starts an isolated new conversation", async () => {
    const user = userEvent.setup();
    render(<CloudAgentChat agent={agent} t={createTranslator("zh")} />);
    await user.type(screen.getByRole("textbox", { name: "聊天消息" }), "hello{Enter}");
    expect(await screen.findByText("hello back")).toBeVisible();
    const first = vi.mocked(sendCloudAgentMessage).mock.calls[0];
    expect(first[0]).toEqual(agent);
    expect(first[1]).toMatchObject({ input: "hello", previousResponseID: "" });
    vi.mocked(sendCloudAgentMessage).mockResolvedValue(completed("resp_second", "continued"));
    await user.type(screen.getByRole("textbox"), "follow up");
    await user.click(screen.getByRole("button", { name: "发送" }));
    expect(await screen.findByText("continued")).toBeVisible();
    expect(vi.mocked(sendCloudAgentMessage).mock.calls[1][1].previousResponseID).toBe("resp_first");
    await user.click(screen.getByRole("button", { name: "新对话" }));
    expect(screen.queryByText("continued")).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox"), "fresh{Enter}");
    await waitFor(() => expect(sendCloudAgentMessage).toHaveBeenCalledTimes(3));
    expect(vi.mocked(sendCloudAgentMessage).mock.calls[2][1].previousResponseID).toBe("");
  });
  it("retries an uncertain send with exactly the same key without duplicating the user message", async () => {
    vi.mocked(sendCloudAgentMessage).mockRejectedValueOnce(new Error("Network unavailable"));
    const user = userEvent.setup();
    render(<CloudAgentChat agent={agent} t={createTranslator("zh")} />);
    await user.type(screen.getByRole("textbox"), "once{Enter}");
    await user.click(await screen.findByRole("button", { name: "重试" }));
    expect(await screen.findByText("hello back")).toBeVisible();
    expect(vi.mocked(sendCloudAgentMessage).mock.calls[0][1].key).toBe(
      vi.mocked(sendCloudAgentMessage).mock.calls[1][1].key,
    );
    expect(screen.getAllByText("once")).toHaveLength(1);
  });
  it("restores pending work by querying its response instead of sending it again", async () => {
    localStorage.setItem(
      key,
      JSON.stringify({
        messages: [{ id: "turn", role: "user", text: "pending" }],
        previousResponseID: "",
        pending: {
          key: "turn",
          input: "pending",
          previousResponseID: "",
          submittedAt: Date.now(),
          responseID: "resp_pending",
        },
      }),
    );
    vi.mocked(fetchCloudAgentResponse).mockResolvedValue(completed("resp_pending", "recovered"));
    render(<CloudAgentChat agent={agent} t={createTranslator("zh")} />);
    expect(await screen.findByText("recovered")).toBeVisible();
    expect(fetchCloudAgentResponse).toHaveBeenCalledWith(agent, "resp_pending");
    expect(sendCloudAgentMessage).not.toHaveBeenCalled();
  });
  it("does not resubmit an old unconfirmed request after the safe retry window", async () => {
    localStorage.setItem(
      key,
      JSON.stringify({
        messages: [{ id: "turn", role: "user", text: "old" }],
        previousResponseID: "",
        pending: { key: "turn", input: "old", previousResponseID: "", submittedAt: 0 },
      }),
    );
    render(<CloudAgentChat agent={agent} t={createTranslator("zh")} />);
    expect(await screen.findByRole("alert")).toBeVisible();
    expect(sendCloudAgentMessage).not.toHaveBeenCalled();
  });
  it("disables sending while offline", () => {
    render(<CloudAgentChat agent={{ ...agent, status: "offline" }} t={createTranslator("zh")} />);
    expect(screen.getByRole("textbox")).toBeDisabled();
    expect(screen.getByText(/智能体当前离线/)).toBeVisible();
  });
  it("keeps Shift+Enter as a newline instead of sending", async () => {
    const user = userEvent.setup();
    render(<CloudAgentChat agent={agent} t={createTranslator("zh")} />);
    await user.type(screen.getByRole("textbox"), "line{Shift>}{Enter}{/Shift}next");
    expect(sendCloudAgentMessage).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox")).toHaveValue("line\nnext");
  });
});

it("sends on an HTTP origin without crypto.randomUUID", async () => {
  const getRandomValues = globalThis.crypto.getRandomValues.bind(globalThis.crypto);
  vi.stubGlobal("crypto", { getRandomValues });
  try {
    render(<CloudAgentChat agent={agent} t={createTranslator("zh")} />);
    await userEvent.type(screen.getByRole("textbox", { name: "聊天消息" }), "hello{Enter}");
    expect(await screen.findByText("hello back")).toBeVisible();
    expect(vi.mocked(sendCloudAgentMessage).mock.calls[0][1].key).toMatch(/^[0-9a-f-]{36}$/);
  } finally {
    vi.unstubAllGlobals();
  }
});
