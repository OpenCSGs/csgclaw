import { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TemplateVersionHistory } from "./TemplateVersionHistory";
import {
  fetchTemplateVersions,
  fetchTemplateVersionWorkspace,
  fetchTemplateVersionFile,
  restoreTemplateVersion,
} from "@/api/hub";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/api/hub", () => ({
  fetchTemplateVersions: vi.fn(),
  fetchTemplateVersionWorkspace: vi.fn(),
  fetchTemplateVersionFile: vi.fn(),
  restoreTemplateVersion: vi.fn(),
}));
const template = { id: "local.Reviewer", uid: "stable-id", revision: 2, name: "Reviewer", source: { kind: "local" } };
const versions = [
  { revision: 1, message: "initial", created_at: "2026-10-29T01:00:00Z" },
  { revision: 2, message: "updated", created_at: "2026-10-29T02:00:00Z", author: { name: "Alice" } },
];
function mount(onRestored = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(workspaceQueryKeys.hubTemplate(template.id), template);
  client.setQueryData(workspaceQueryKeys.hubTemplates(), [template]);
  const view = render(
    <QueryClientProvider client={client}>
      <TemplateVersionHistory template={template} locale="en" t={createTranslator("en")} onRestored={onRestored} />
    </QueryClientProvider>,
  );
  const summary = view.container.querySelector("details")!;
  summary.open = true;
  fireEvent(summary, new Event("toggle"));
  return { view, onRestored, client };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchTemplateVersions).mockResolvedValue(versions);
  vi.mocked(fetchTemplateVersionWorkspace).mockResolvedValue({
    kind: "dir",
    path: "",
    entries: [{ name: "agent.toml", path: "agent.toml", type: "file" }],
  });
  vi.mocked(fetchTemplateVersionFile).mockResolvedValue({ path: "agent.toml", content: 'name = "Reviewer"', size: 17 });
  vi.mocked(restoreTemplateVersion).mockResolvedValue({ ...template, revision: 1, latest_revision: 2 });
});
describe("TemplateVersionHistory", () => {
  it("previews history and rolls back from the latest version without creating a revision", async () => {
    const user = userEvent.setup();
    const { onRestored, client } = mount();
    await user.click(await screen.findByRole("button", { name: "V1" }));
    expect(screen.getByRole("button", { name: "Switch version" })).toBeEnabled();
    expect(screen.getByText("Active V2")).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "agent.toml" }));
    expect(await screen.findByText('name = "Reviewer"')).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "V2" }));
    await user.click(await screen.findByRole("button", { name: "Switch version" }));
    expect(screen.getByRole("combobox", { name: "Target version" })).toHaveTextContent("V1");
    await user.click(screen.getByRole("button", { name: "Confirm switch to V1" }));
    await waitFor(() => expect(restoreTemplateVersion).toHaveBeenCalledWith("local.Reviewer", 1));
    await waitFor(() =>
      expect(onRestored).toHaveBeenCalledWith(expect.objectContaining({ revision: 1, latest_revision: 2 })),
    );
    expect(screen.getByRole("button", { name: "V2" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "V3" })).not.toBeInTheDocument();
    expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
      expect.objectContaining({ revision: 1, latest_revision: 2 }),
    );
  });
  it("switches from V2 to V1 and back to V2 without losing history", async () => {
    const user = userEvent.setup();
    vi.mocked(restoreTemplateVersion).mockImplementation(async (_id, revision) => ({
      ...template,
      revision,
      latest_revision: 2,
    }));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    function Harness() {
      const [active, setActive] = useState({ ...template, latest_revision: 2 });
      return (
        <TemplateVersionHistory
          template={active}
          locale="en"
          t={createTranslator("en")}
          onRestored={(item) =>
            setActive({ ...template, revision: item.revision!, latest_revision: item.latest_revision! })
          }
        />
      );
    }
    const view = render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>,
    );
    const summary = view.container.querySelector("details")!;
    summary.open = true;
    fireEvent(summary, new Event("toggle"));
    await user.click(await screen.findByRole("button", { name: "V1" }));
    await user.click(screen.getByRole("button", { name: "Switch version" }));
    expect(screen.getByRole("combobox", { name: "Target version" })).toHaveTextContent("V1");
    await user.click(screen.getByRole("button", { name: "Confirm switch to V1" }));
    await waitFor(() =>
      expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
        expect.objectContaining({ revision: 1 }),
      ),
    );
    await user.click(await screen.findByRole("button", { name: "V2" }));
    await user.click(screen.getByRole("button", { name: "Switch version" }));
    expect(screen.getByRole("combobox", { name: "Target version" })).toHaveTextContent("V2");
    await user.click(screen.getByRole("button", { name: "Confirm switch to V2" }));
    await waitFor(() =>
      expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
        expect.objectContaining({ revision: 2, latest_revision: 2 }),
      ),
    );
    expect(restoreTemplateVersion).toHaveBeenNthCalledWith(1, template.id, 1);
    expect(restoreTemplateVersion).toHaveBeenNthCalledWith(2, template.id, 2);
    expect(screen.getByRole("button", { name: "V1" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "V3" })).not.toBeInTheDocument();
  });
  it("applies a successful switch without waiting for a history refresh", async () => {
    const user = userEvent.setup();
    const { client, onRestored } = mount();
    await user.click(await screen.findByRole("button", { name: "V1" }));
    vi.mocked(fetchTemplateVersions).mockRejectedValue(new Error("history unavailable"));
    await user.click(screen.getByRole("button", { name: "Switch version" }));
    await user.click(screen.getByRole("button", { name: "Confirm switch to V1" }));
    await waitFor(() =>
      expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
        expect.objectContaining({ revision: 1 }),
      ),
    );
    expect(onRestored).toHaveBeenCalledWith(expect.objectContaining({ revision: 1 }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Confirm switch to V1" })).not.toBeInTheDocument();
  });
  it("clears an old preview while loading a different version", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: "V1" }));
    await user.click(await screen.findByRole("button", { name: "agent.toml" }));
    expect(await screen.findByText('name = "Reviewer"')).toBeInTheDocument();
    let resolveListing!: (value: { kind: string; path: string; entries: [] }) => void;
    vi.mocked(fetchTemplateVersionWorkspace).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveListing = resolve;
      }),
    );
    await user.click(screen.getByRole("button", { name: "V2" }));
    expect(screen.queryByText('name = "Reviewer"')).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "V2" })).toBeDisabled();
    resolveListing({ kind: "dir", path: "", entries: [] });
    expect(await screen.findByText("This directory is empty.")).toBeInTheDocument();
  });
  it("shows an empty history for legacy templates", async () => {
    vi.mocked(fetchTemplateVersions).mockResolvedValue([]);
    mount();
    expect(
      await screen.findByText("No versions yet. History starts when the agent is next saved."),
    ).toBeInTheDocument();
  });
});
