import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SkillUpdateDialog } from "./SkillUpdateDialog";
import { createTranslator } from "@/shared/i18n";
import { createQueryWrapper } from "../../../../tests/helpers/queryClient";
import type { SkillUpdateInfo } from "@/models/skillUpdates";

function info(name: string, version = "1.1.0"): SkillUpdateInfo {
  return {
    name,
    available: true,
    localVersion: "1.0.0",
    latestVersion: version,
    commit: `commit-${version}`,
    state: `state-${name}`,
    publishedAt: "2026-11-13T00:00:00Z",
    checkedAt: "2026-11-13T01:00:00Z",
    sourceUrl: `https://example.com/skills/team/${name}`,
    remotePath: `team/${name}`,
    changelog: `Changes for ${name}`,
    targets: [
      {
        id: "library",
        name: "",
        version: "1.0.0",
        state: "target-state",
        changes: [
          { path: "SKILL.md", kind: "modified", before: "My custom instructions", after: "New published instructions" },
        ],
      },
      { id: "agent-1", name: "Writer Agent", version: "0.9.0", state: "agent-state", changes: [] },
    ],
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

it.each(["zh", "en"] as const)(
  "shows every version transition and defaults to updating with one confirmation in %s",
  async (locale) => {
    const t = createTranslator(locale);
    const plans = [info("research"), info("writer", "2.0.0")];
    const fetch = vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "POST")
        return Response.json({
          results: plans.map((item) => ({ name: item.name, targetId: "library", status: "updated" })),
        });
      const name = String(input).includes("research") ? "research" : "writer";
      return Response.json(plans.find((item) => item.name === name));
    });
    vi.stubGlobal("fetch", fetch);
    const onClose = vi.fn();
    const { wrapper } = createQueryWrapper();
    render(<SkillUpdateDialog names={["research", "writer"]} t={t} onClose={onClose} />, { wrapper });
    await screen.findAllByText("v2.0.0");
    for (const plan of plans) {
      const row = within(screen.getByRole("region", { name: plan.name }));
      expect(row.getAllByText("v1.0.0").length).toBeGreaterThan(0);
      expect(row.getAllByText(`v${plan.latestVersion}`).length).toBeGreaterThan(0);
      expect(row.getByText("v0.9.0")).toBeInTheDocument();
    }
    const confirm = screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") });
    expect(confirm).toBeEnabled();
    expect(screen.getByRole("radio", { name: new RegExp(`^${t("skillUpdatesChoiceUpdate")}`) })).toBeChecked();
    expect(screen.getByRole("radio", { name: new RegExp(`^${t("skillUpdatesChoiceKeep")}`) })).not.toBeChecked();
    expect(fetch.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false);
    const user = userEvent.setup();
    await user.click(confirm);
    await screen.findAllByText(t("skillUpdatesDone"));
    const mutation = fetch.mock.calls.find(([, options]) => options?.method === "POST");
    expect(JSON.parse(String(mutation?.[1]?.body))).toEqual({
      items: plans.map((plan) => ({
        name: plan.name,
        version: plan.latestVersion,
        commit: plan.commit,
        state: plan.state,
        strategy: "update",
      })),
    });
    await user.click(screen.getAllByRole("button", { name: t("close") }).at(-1)!);
    expect(onClose).toHaveBeenCalledWith(true);
  },
);

it("cancels without a mutation and can ignore a release without overwriting", async () => {
  const t = createTranslator("en");
  const fetch = vi.fn(async (_input: string, init?: RequestInit) =>
    init?.method === "POST"
      ? Response.json({ results: [{ name: "writer", status: "kept" }] })
      : Response.json(info("writer")),
  );
  vi.stubGlobal("fetch", fetch);
  const onClose = vi.fn();
  const { wrapper } = createQueryWrapper();
  const { unmount } = render(<SkillUpdateDialog names={["writer"]} t={t} onClose={onClose} />, { wrapper });
  await screen.findByRole("radio", { name: new RegExp(`^${t("skillUpdatesChoiceKeep")}`) });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: t("cancel") }));
  expect(onClose).toHaveBeenCalledWith(false);
  expect(fetch.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false);
  unmount();
  render(<SkillUpdateDialog names={["writer"]} t={t} onClose={onClose} />, { wrapper });
  await user.click(await screen.findByRole("radio", { name: new RegExp(`^${t("skillUpdatesChoiceKeep")}`) }));
  await user.click(screen.getByRole("button", { name: t("skillUpdatesConfirmKeep") }));
  await screen.findByText(t("skillUpdatesKept"));
  expect(
    JSON.parse(String(fetch.mock.calls.find(([, options]) => options?.method === "POST")?.[1]?.body)).items[0].strategy,
  ).toBe("keep-local");
});

it("requires a fresh preview after a stale confirmation and restores the default update choice", async () => {
  const t = createTranslator("en");
  const fetch = vi.fn(async (_input: string, init?: RequestInit) =>
    init?.method === "POST"
      ? Response.json({ results: [{ name: "writer", status: "failed", error: "skill_installation_changed" }] })
      : Response.json(info("writer")),
  );
  vi.stubGlobal("fetch", fetch);
  const { wrapper } = createQueryWrapper();
  render(<SkillUpdateDialog names={["writer"]} t={t} onClose={vi.fn()} />, { wrapper });
  const user = userEvent.setup();
  await user.click(await screen.findByRole("radio", { name: new RegExp(`^${t("skillUpdatesChoiceUpdate")}`) }));
  await user.click(screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") }));
  await screen.findByText(t("skillUpdatesChanged"));
  await user.click(screen.getByRole("button", { name: t("skillUpdatesCheckAgain") }));
  await waitFor(() => expect(screen.getAllByRole("radio")).toHaveLength(2));
  await waitFor(() => expect(screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") })).toBeEnabled());
  expect(screen.getByRole("radio", { name: new RegExp(`^${t("skillUpdatesChoiceUpdate")}`) })).toBeChecked();
});

it("labels the same version as a reinstall and cancels without changing the installation", async () => {
  const t = createTranslator("en");
  const plan = info("writer", "1.0.0");
  plan.targets = plan.targets.map((target) => ({ ...target, version: "1.0.0" }));
  const fetch = vi.fn(async () => Response.json(plan));
  vi.stubGlobal("fetch", fetch);
  const onClose = vi.fn();
  const { wrapper } = createQueryWrapper();
  render(<SkillUpdateDialog names={["writer"]} t={t} onClose={onClose} />, { wrapper });
  await screen.findByRole("dialog", { name: t("skillUpdatesReinstallTitle", { count: 1 }) });
  expect(screen.queryByText(t("skillUpdatesReleaseNotes"))).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("radio", { name: new RegExp(`^${t("skillUpdatesReinstallKeep")}`) }));
  await user.click(screen.getByRole("button", { name: t("skillUpdatesConfirmKeep") }));
  expect(onClose).toHaveBeenCalledWith(false);
  expect(fetch).toHaveBeenCalledOnce();
});

it("keeps an unknown installed version explicit when reviewing a published update", async () => {
  const t = createTranslator("zh");
  const plan = info("unversioned-skill");
  plan.localVersion = "";
  plan.targets = plan.targets.map((target) => ({ ...target, version: "" }));
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(plan)),
  );
  const { wrapper } = createQueryWrapper();
  render(<SkillUpdateDialog names={[plan.name]} t={t} onClose={vi.fn()} />, { wrapper });
  await screen.findAllByText(t("iaSkillsUnversioned"));
  expect(screen.getAllByText("v1.1.0").length).toBeGreaterThan(0);
  expect(screen.queryByText("v0.0.0")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") })).toBeEnabled();
});

it("does not offer a reinstall when all copies are newer than the published release", async () => {
  const t = createTranslator("en");
  const plan = { ...info("writer"), available: false, localVersion: "3.0.0", targets: [] };
  const fetch = vi.fn(async () => Response.json(plan));
  vi.stubGlobal("fetch", fetch);
  const { wrapper } = createQueryWrapper();
  render(<SkillUpdateDialog names={["writer"]} t={t} onClose={vi.fn()} />, { wrapper });
  await screen.findByText(t("skillUpdatesNewerCopies"));
  expect(screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") })).toBeDisabled();
  expect(fetch).toHaveBeenCalledOnce();
});

it("describes the actual affected copies when the newer library is excluded", async () => {
  const t = createTranslator("en");
  const plan = info("writer");
  plan.localVersion = "3.0.0";
  plan.targets = plan.targets.filter((target) => target.id !== "library");
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(plan)),
  );
  const { wrapper } = createQueryWrapper();
  render(<SkillUpdateDialog names={["writer"]} t={t} onClose={vi.fn()} />, { wrapper });
  await screen.findByText(t("skillUpdatesImpact", { count: 1 }));
  expect(screen.queryByText("v3.0.0")).not.toBeInTheDocument();
  expect(screen.getByText("Writer Agent")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") })).toBeEnabled();
});

it("explains a stale preview error returned by the API", async () => {
  const t = createTranslator("en");
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ error: { code: "skill_installation_changed" } }, { status: 409 })),
  );
  const { wrapper } = createQueryWrapper();
  render(<SkillUpdateDialog names={["writer"]} t={t} onClose={vi.fn()} />, { wrapper });
  await screen.findByRole("alert");
  expect(screen.getByText(t("skillUpdatesChanged"))).toBeInTheDocument();
  expect(screen.getByRole("button", { name: t("skillUpdatesConfirmUpdate") })).toBeDisabled();
});
