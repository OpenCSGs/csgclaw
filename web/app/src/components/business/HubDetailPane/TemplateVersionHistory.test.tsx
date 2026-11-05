import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TemplateVersionHistory } from "./TemplateVersionHistory";
import {
  fetchTemplateVersions,
  fetchTemplateVersionWorkspace,
  fetchTemplateVersionFile,
  restoreTemplateVersion,
  deleteTemplateVersion,
} from "@/api/hub";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { createTranslator } from "@/shared/i18n";

vi.mock("@/api/hub", () => ({
  fetchTemplateVersions: vi.fn(),
  fetchTemplateVersionWorkspace: vi.fn(),
  fetchTemplateVersionFile: vi.fn(),
  restoreTemplateVersion: vi.fn(),
  deleteTemplateVersion: vi.fn(),
}));
const template = { id: "local.Reviewer", uid: "stable-id", revision: 2, name: "Reviewer", source: { kind: "local" } };
const versions = [
  { revision: 1, message: "initial", created_at: "2026-10-29T01:00:00Z" },
  { revision: 2, message: "updated", created_at: "2026-10-29T02:00:00Z", author: { name: "Alice" } },
];
function mount(onRestored = vi.fn(), selectedTemplate = template) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(workspaceQueryKeys.hubTemplate(selectedTemplate.id), selectedTemplate);
  client.setQueryData(workspaceQueryKeys.hubTemplates(), [selectedTemplate]);
  const view = render(
    <QueryClientProvider client={client}>
      <TemplateVersionHistory
        template={selectedTemplate}
        locale="en"
        t={createTranslator("en")}
        onRestored={onRestored}
      />
    </QueryClientProvider>,
  );
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
  it("defaults to the active version even when a newer version exists, without activating anything", async () => {
    mount(vi.fn(), { ...template, revision: 1 });
    expect(await screen.findByRole("button", { name: "agent.toml" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "V1 · Active" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("region", { name: "V1 preview" })).toBeInTheDocument();
    expect(fetchTemplateVersionWorkspace).toHaveBeenCalledExactlyOnceWith(template.id, 1, "");
    expect(restoreTemplateVersion).not.toHaveBeenCalled();
  });
  it("preserves a manually selected history version when the parent rerenders", async () => {
    const user = userEvent.setup();
    const { view, client } = mount();
    await screen.findByRole("button", { name: "agent.toml" });
    await user.click(screen.getByRole("button", { name: "V1" }));
    view.rerender(
      <QueryClientProvider client={client}>
        <TemplateVersionHistory template={{ ...template }} locale="en" t={createTranslator("en")} />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("region", { name: "V1 preview" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "V1" })).toHaveAttribute("aria-current", "true");
    expect(fetchTemplateVersions).toHaveBeenCalledTimes(1);
    expect(fetchTemplateVersionWorkspace).toHaveBeenCalledTimes(2);
  });
  it("leaves the choice open when the active version is missing from history", async () => {
    vi.mocked(fetchTemplateVersions).mockResolvedValue([versions[0]]);
    mount();
    expect(await screen.findByRole("button", { name: "V1" })).not.toHaveAttribute("aria-current");
    expect(screen.getByRole("region", { name: "Version content" })).toBeInTheDocument();
    expect(fetchTemplateVersionWorkspace).not.toHaveBeenCalled();
    expect(restoreTemplateVersion).not.toHaveBeenCalled();
  });
  it("shows a default preview failure and retries the selected version", async () => {
    const user = userEvent.setup();
    vi.mocked(fetchTemplateVersionWorkspace).mockRejectedValueOnce(new Error("Preview unavailable"));
    mount();
    expect(await screen.findByRole("alert")).toHaveTextContent("Preview unavailable");
    expect(screen.queryByText("This directory is empty.")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "V2 · Active" }));
    expect(await screen.findByRole("button", { name: "agent.toml" })).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(restoreTemplateVersion).not.toHaveBeenCalled();
  });
  it("keeps summaries visible and confirms deletion from the inactive version menu", async () => {
    const user = userEvent.setup();
    vi.mocked(deleteTemplateVersion).mockResolvedValue({ deleted: true });
    const { client } = mount();
    expect(await screen.findByRole("button", { name: /^V1(?: · Active)?$/ })).not.toHaveAttribute("aria-current");
    expect(screen.getByText("initial")).toBeInTheDocument();
    expect(screen.getByText("updated")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "More actions for V2" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete version V2" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "More actions for V1" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete version V1" }));
    expect(deleteTemplateVersion).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Delete version V1" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /^V1(?: · Active)?$/ })).not.toBeInTheDocument());
    expect(deleteTemplateVersion).toHaveBeenCalledWith(template.id, 1);
    expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
      expect.objectContaining({ version_count: 1 }),
    );
  });
  it("previews and activates the selected history without creating a revision", async () => {
    const user = userEvent.setup();
    const { onRestored, client } = mount();
    await user.click(await screen.findByRole("button", { name: /^V1(?: · Active)?$/ }));
    expect(screen.getByRole("button", { name: "Set as active version" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "V2 · Active" })).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "agent.toml" }));
    expect(await screen.findByText('name = "Reviewer"')).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Set as active version" }));
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.getByText("Switch the active version from V2 to V1.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Confirm switch to V1" }));
    await waitFor(() => expect(restoreTemplateVersion).toHaveBeenCalledWith("local.Reviewer", 1));
    await waitFor(() =>
      expect(onRestored).toHaveBeenCalledWith(expect.objectContaining({ revision: 1, latest_revision: 2 })),
    );
    expect(screen.getByRole("button", { name: /^V2(?: · Active)?$/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^V3(?: · Active)?$/ })).not.toBeInTheDocument();
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
    render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>,
    );
    await user.click(await screen.findByRole("button", { name: /^V1(?: · Active)?$/ }));
    await user.click(screen.getByRole("button", { name: "Set as active version" }));
    expect(screen.getByText("Switch the active version from V2 to V1.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Confirm switch to V1" }));
    await waitFor(() =>
      expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
        expect.objectContaining({ revision: 1 }),
      ),
    );
    await user.click(await screen.findByRole("button", { name: /^V2(?: · Active)?$/ }));
    await user.click(screen.getByRole("button", { name: "Set as active version" }));
    expect(screen.getByText("Switch the active version from V1 to V2.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Confirm switch to V2" }));
    await waitFor(() =>
      expect(client.getQueryData(workspaceQueryKeys.hubTemplate(template.id))).toEqual(
        expect.objectContaining({ revision: 2, latest_revision: 2 }),
      ),
    );
    expect(restoreTemplateVersion).toHaveBeenNthCalledWith(1, template.id, 1);
    expect(restoreTemplateVersion).toHaveBeenNthCalledWith(2, template.id, 2);
    expect(screen.getByRole("button", { name: /^V1(?: · Active)?$/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^V3(?: · Active)?$/ })).not.toBeInTheDocument();
  });
  it("applies a successful switch without waiting for a history refresh", async () => {
    const user = userEvent.setup();
    const { client, onRestored } = mount();
    await user.click(await screen.findByRole("button", { name: /^V1(?: · Active)?$/ }));
    vi.mocked(fetchTemplateVersions).mockRejectedValue(new Error("history unavailable"));
    await user.click(screen.getByRole("button", { name: "Set as active version" }));
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
    await user.click(await screen.findByRole("button", { name: /^V1(?: · Active)?$/ }));
    await user.click(await screen.findByRole("button", { name: "agent.toml" }));
    expect(await screen.findByText('name = "Reviewer"')).toBeInTheDocument();
    let resolveListing!: (value: { kind: string; path: string; entries: [] }) => void;
    vi.mocked(fetchTemplateVersionWorkspace).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveListing = resolve;
      }),
    );
    await user.click(screen.getByRole("button", { name: /^V2(?: · Active)?$/ }));
    expect(screen.queryByText('name = "Reviewer"')).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^V2(?: · Active)?$/ })).toBeDisabled();
    resolveListing({ kind: "dir", path: "", entries: [] });
    expect(await screen.findByText("This directory is empty.")).toBeInTheDocument();
  });
  it("does not offer to switch the already active version", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: /^V2 · Active$/ }));
    expect(screen.getAllByText("Active")).toHaveLength(1);
    expect(screen.queryByText("Currently in use")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Set as active version" })).not.toBeInTheDocument();
    expect(screen.getByText("initial")).toBeInTheDocument();
    expect(restoreTemplateVersion).not.toHaveBeenCalled();
  });
  it("cancels a switch without changing the selected preview", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: "V1" }));
    await user.click(screen.getByRole("button", { name: "Set as active version" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "V1 preview" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Set as active version" })).toHaveFocus();
    expect(restoreTemplateVersion).not.toHaveBeenCalled();
  });
  it("keeps failed deletion in the confirmation dialog for retry", async () => {
    const user = userEvent.setup();
    vi.mocked(deleteTemplateVersion).mockRejectedValueOnce(new Error("Deletion failed"));
    mount();
    await user.click(await screen.findByRole("button", { name: "More actions for V1" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete version V1" }));
    await user.click(screen.getByRole("button", { name: "Delete version V1" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Deletion failed");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    vi.mocked(deleteTemplateVersion).mockResolvedValueOnce({ deleted: true });
    await user.click(screen.getByRole("button", { name: "Delete version V1" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "V1" })).not.toBeInTheDocument();
  });
  it("shows an empty history for legacy templates", async () => {
    vi.mocked(fetchTemplateVersions).mockResolvedValue([]);
    mount();
    expect(
      await screen.findByText("No versions yet. History starts when the agent is next saved."),
    ).toBeInTheDocument();
  });
});
