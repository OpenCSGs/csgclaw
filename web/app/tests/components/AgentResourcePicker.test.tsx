import type { PropsWithChildren } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { AgentResourcePicker } from "@/components/business/AgentDetails/AgentResourcePicker";
import { createTranslator } from "@/shared/i18n";
import { createQueryWrapper } from "../helpers/queryClient";

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

function createPickerWrapper() {
  const { wrapper: QueryWrapper } = createQueryWrapper();
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <MemoryRouter initialEntries={["/agents/agent-1?tab=skills&view=scheduled"]}>
        <QueryWrapper>
          {children}
          <LocationProbe />
        </QueryWrapper>
      </MemoryRouter>
    );
  };
}

function setup(
  locale: "en" | "zh" = "en",
  fileFailure = false,
  existingSkill: "none" | "global" | "enabled" | "disabled" = "none",
  installedName = "review",
) {
  const t = createTranslator(locale);
  let installed = existingSkill !== "none";
  const fetch = vi.fn(async (input: string, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost/");
    const path = url.pathname;
    if (path === "/api/v1/skills") return Response.json(installed ? [{ name: installedName }] : []);
    if (path.endsWith("/skill-summaries")) {
      return Response.json(
        existingSkill === "enabled" || existingSkill === "disabled"
          ? [{ name: installedName, enabled: existingSkill === "enabled" }]
          : [],
      );
    }
    if (path === "/api/v1/skills/remote") {
      return Response.json({
        items: [{ name: "Review", description: "Review changes", remote_path: "team/review", remote_ref: "main" }],
        total: 1,
      });
    }
    if (path === "/api/v1/skills/tree") {
      return Response.json({
        kind: "dir",
        path: "",
        entries: [
          { name: "SKILL.md", path: "SKILL.md", kind: "file" },
          { name: "guide.md", path: "guide.md", kind: "file" },
        ],
      });
    }
    if (path === "/api/v1/skills/file") {
      if (fileFailure) {
        fileFailure = false;
        return Response.json({ error: "File temporarily unavailable" }, { status: 503 });
      }
      const filePath = url.searchParams.get("path");
      return Response.json({
        path: filePath,
        content: filePath === "SKILL.md" ? "Review instructions" : "Detailed guide",
      });
    }
    if (path === "/api/v1/skills:install" && init?.method === "POST") {
      installed = true;
      return Response.json({ name: installedName });
    }
    throw new Error(`Unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetch);
  const onAdd = vi.fn().mockResolvedValue(true);
  const onClose = vi.fn();
  render(<AgentResourcePicker agentID="agent-1" kind="skill" authenticated onAdd={onAdd} onClose={onClose} t={t} />, {
    wrapper: createPickerWrapper(),
  });
  return { fetch, onAdd, onClose, t, user: userEvent.setup() };
}

afterEach(() => vi.unstubAllGlobals());

describe("Agent global resource picker", () => {
  it.each([
    ["skill", "profileManageSkills", "skills"],
    ["mcp", "profileManageMCP", "connectors"],
    ["knowledge", "profileManageKnowledge", "knowledge"],
  ] as const)("opens %s management in Settings while preserving the Agent location", async (kind, label, section) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json(kind === "skill" ? [] : {})),
    );
    const t = createTranslator("en");
    const onClose = vi.fn();
    render(
      <AgentResourcePicker
        agentID="agent-1"
        kind={kind}
        authenticated={false}
        onAdd={vi.fn()}
        onClose={onClose}
        t={t}
      />,
      { wrapper: createPickerWrapper() },
    );
    await userEvent.setup().click(screen.getByRole("button", { name: t(label) }));
    expect(onClose).toHaveBeenCalledOnce();
    const destination = new URL(screen.getByTestId("location").textContent || "", "http://localhost");
    expect(destination.pathname).toBe("/agents/agent-1");
    expect(Object.fromEntries(destination.searchParams)).toEqual({
      tab: "skills",
      view: "scheduled",
      dialog: "settings",
      section,
    });
  });

  it.each(["mcp", "knowledge"] as const)("allows a disabled %s resource to be enabled again", async (kind) => {
    const config =
      kind === "knowledge"
        ? {
            url: "https://example.com/wiki/mcp",
            _meta: {
              "com.opencsg/mcp": {
                type: "llm_wiki",
                resource_id: "42",
                content_id: "wiki-42",
                auth_type: "csghub_access_token",
              },
            },
          }
        : { command: "node" };
    const servers = {
      disabled: { ...config, display_name: "Disabled resource", enabled: false },
      enabled: { ...config, display_name: "Enabled resource" },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (String(input).includes("/agents/")) return Response.json({ servers });
        return Response.json({ mcpServers: servers });
      }),
    );
    const t = createTranslator("en");
    const onAdd = vi.fn().mockResolvedValue(true);
    const onClose = vi.fn();
    render(
      <AgentResourcePicker agentID="agent-1" kind={kind} authenticated={false} onAdd={onAdd} onClose={onClose} t={t} />,
      { wrapper: createPickerWrapper() },
    );
    const disabled = await screen.findByRole("checkbox", { name: "Disabled resource" });
    expect(disabled).not.toBeChecked();
    expect(disabled).toBeEnabled();
    const enabled = screen.getByRole("checkbox", { name: "Enabled resource" });
    expect(enabled).toBeChecked();
    expect(enabled).toBeDisabled();
    const user = userEvent.setup();
    await user.click(disabled);
    await user.click(screen.getByRole("button", { name: t(kind === "mcp" ? "agentMCPAdd" : "agentKnowledgeAdd") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledExactlyOnceWith(["disabled"]));
    expect(onClose).toHaveBeenCalledOnce();
  });
});

describe("Agent skill market preview", () => {
  it.each(["en", "zh"] as const)("replaces the existing global skill from the preview in %s", async (locale) => {
    const { fetch, onAdd, onClose, t, user } = setup(locale, false, "global");
    await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
    await screen.findByRole("button", { name: "Review" });
    expect(screen.queryByRole("button", { name: t("profileResourceEnabled") })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: t("iaSkillsUpdate") })).toBeEnabled();
    expect(screen.getByRole("button", { name: t("iaSkillsUpdate") })).toHaveClass("btn-secondary-gray");
    await user.click(screen.getByRole("button", { name: "Review" }));
    const preview = within(screen.getByRole("dialog", { name: "Review" }));
    await preview.findByText("Review instructions");
    expect(preview.getByRole("button", { name: t("iaSkillsUpdate") })).toHaveClass("btn-secondary-gray");
    await user.click(preview.getByRole("button", { name: t("iaSkillsUpdate") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true }));
    expect(onClose).not.toHaveBeenCalled();
    expect(fetch.mock.calls.find(([path]) => String(path) === "api/v1/skills:install")?.[1]?.body).toBe(
      JSON.stringify({ remote_path: "team/review", ref: "main", replace: true }),
    );
  });

  it("allows an enabled worker skill to be replaced from the list and preview", async () => {
    const { fetch, onAdd, t, user } = setup("en", false, "enabled");
    await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
    await screen.findByRole("button", { name: "Review" });
    expect(screen.getByRole("button", { name: t("iaSkillsUpdate") })).toBeEnabled();
    expect(screen.getByRole("button", { name: t("iaSkillsUpdate") })).toHaveClass("btn-secondary-gray");
    await user.click(screen.getByRole("button", { name: "Review" }));
    const preview = within(screen.getByRole("dialog", { name: "Review" }));
    await user.click(preview.getByRole("button", { name: t("iaSkillsUpdate") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true }));
    expect(fetch.mock.calls.find(([path]) => String(path) === "api/v1/skills:install")?.[1]?.body).toBe(
      JSON.stringify({ remote_path: "team/review", ref: "main", replace: true }),
    );
  });

  it("allows a disabled skill to be selected in the library and replaced from the market", async () => {
    const { fetch, onAdd, t, user } = setup("en", false, "disabled");
    const checkbox = await screen.findByRole("checkbox", { name: "review" });
    expect(checkbox).toBeEnabled();
    expect(checkbox).not.toBeChecked();
    await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
    await screen.findByRole("button", { name: "Review" });
    await user.click(screen.getByRole("button", { name: t("iaSkillsUpdate") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledExactlyOnceWith(["review"], { replace: true }));
    expect(fetch.mock.calls.find(([path]) => String(path) === "api/v1/skills:install")?.[1]?.body).toBe(
      JSON.stringify({ remote_path: "team/review", ref: "main", replace: true }),
    );
  });

  it("shows replace after installation and retries a worker failure without reinstalling", async () => {
    const { fetch, onAdd, onClose, t, user } = setup();
    onAdd.mockResolvedValueOnce(false);
    await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
    await screen.findByRole("button", { name: "Review" });
    await user.click(screen.getByRole("button", { name: t("resourcesSkillRemoteInstallAction") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledOnce());
    expect(onClose).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Review" }));
    const preview = within(screen.getByRole("dialog", { name: "Review" }));
    await user.click(preview.getByRole("button", { name: t("iaSkillsUpdate") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledTimes(2));
    expect(onClose).not.toHaveBeenCalled();
    expect(fetch.mock.calls.filter(([path]) => String(path) === "api/v1/skills:install")).toHaveLength(1);
  });

  it.each(["en", "zh"] as const)(
    "opens remote files with keyboard navigation in %s and retains the market search",
    async (locale) => {
      const { fetch, t, user } = setup(locale);
      await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
      await user.type(screen.getByRole("searchbox"), "Review");
      const item = await screen.findByRole("button", { name: "Review" });
      item.focus();
      await user.keyboard("{Enter}");
      const preview = within(await screen.findByRole("dialog", { name: "Review" }));
      await preview.findByText("Review instructions");
      await user.click(preview.getByRole("button", { name: "guide.md" }));
      await preview.findByText("Detailed guide");
      const fileRequests = fetch.mock.calls.filter(([path]) => String(path).includes("/skills/file?"));
      expect(fileRequests).toHaveLength(2);
      for (const [path] of fileRequests) {
        const params = new URL(String(path), "http://localhost/").searchParams;
        expect(params.get("remote_path")).toBe("team/review");
        expect(params.get("ref")).toBe("main");
      }
      expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
      await user.keyboard("{Escape}");
      expect(screen.queryByRole("dialog", { name: "Review" })).not.toBeInTheDocument();
      expect(screen.getByRole("searchbox")).toHaveValue("Review");
      await waitFor(() => expect(item).toHaveFocus());
    },
  );

  it("installs from the preview and enables the canonical skill name for the Agent", async () => {
    const { onAdd, onClose, t, user } = setup("en", false, "none", "custom-review");
    await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
    await user.click(await screen.findByRole("button", { name: "Review" }));
    const preview = within(screen.getByRole("dialog", { name: "Review" }));
    await preview.findByText("Review instructions");
    await user.click(preview.getByRole("button", { name: t("resourcesSkillRemoteInstallAction") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledWith(["custom-review"], { replace: true }));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("keeps the row install action independent of preview and retries file failures", async () => {
    const { onAdd, onClose, t, user } = setup("en", true);
    await user.click(screen.getByRole("radio", { name: t("profileSkillMarket") }));
    await user.click(await screen.findByRole("button", { name: "Review" }));
    const preview = within(screen.getByRole("dialog", { name: "Review" }));
    await preview.findByRole("alert");
    await user.click(preview.getByRole("button", { name: t("retry") }));
    await preview.findByText("Review instructions");
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: t("resourcesSkillRemoteInstallAction") }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledWith(["review"], { replace: true }));
    expect(screen.queryByRole("dialog", { name: "Review" })).not.toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("radio", { name: t("profileSkillMarket") })).toBeChecked();
    expect(screen.getByRole("button", { name: t("iaSkillsUpdate") })).toHaveClass("btn-secondary-gray");
  });
});
