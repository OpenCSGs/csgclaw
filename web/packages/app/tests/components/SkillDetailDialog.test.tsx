import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { fetchSkillFile, fetchSkillTree } from "@/api/skills";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import { SkillDetailDialog, type SkillDetailTarget } from "@/pages/SettingsPage/sections/SkillDetailDialog";
import { createTranslator } from "@/shared/i18n";
import type { SkillFile } from "@/models/skillhub";

vi.mock("@/api/skills", () => ({ fetchSkillFile: vi.fn(), fetchSkillTree: vi.fn() }));
const t = createTranslator("zh");
const onSelectSkillInPlace = vi.fn();

function setup(target: SkillDetailTarget) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <WorkspaceControllerProvider
        controller={
          {
            ready: true,
            t,
            hubViewProps: {
              hub: {
                skillTreeError: "stale global tree error",
                skillFileError: "stale global file error",
                detailPaneProps: { onSelectSkillInPlace },
              },
            },
          } as never
        }
      >
        {children}
      </WorkspaceControllerProvider>
    </QueryClientProvider>
  );
  return render(<SkillDetailDialog target={target} t={t} onOpenChange={vi.fn()} />, { wrapper });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(fetchSkillTree).mockResolvedValue({ kind: "dir", path: "", entries: [] });
  vi.mocked(fetchSkillFile).mockResolvedValue({ path: "SKILL.md", content: "Skill instructions" });
});

describe("skill details", () => {
  it.each([
    ["unknown", "本地"],
    ["local", "本地上传"],
    ["market", "技能市场"],
  ])("labels a %s installation without inventing its history", async (source, label) => {
    setup({ variant: "installed", skill: { name: "demo", source } });
    const badge = screen.getByText(label);
    if (source === "unknown") {
      expect(badge).toHaveAttribute("title", t("iaSkillsBadgeUntrackedHint"));
    } else {
      expect(badge).not.toHaveAttribute("title");
    }
    await screen.findByText("Skill instructions");
  });

  it("loads remote files by repository and ref even when the display name differs", async () => {
    const skill = { name: "媒体生成", remotePath: "AIWizards/media-generator", remoteRef: "dev", source: "official" };
    setup({ variant: "market", skill });
    await screen.findByText("Skill instructions");
    expect(fetchSkillTree).toHaveBeenCalledWith("", skill);
    expect(fetchSkillFile).toHaveBeenCalledWith("SKILL.md", skill);
    expect(onSelectSkillInPlace).not.toHaveBeenCalled();
    expect(screen.queryByText(/stale global/)).not.toBeInTheDocument();
  });

  it("recovers from a failed market detail when opening another or an installed skill", async () => {
    vi.mocked(fetchSkillFile).mockRejectedValueOnce(new Error("First remote unavailable"));
    const view = setup({ variant: "market", skill: { name: "first", remotePath: "team/first" } });
    expect(await screen.findByRole("alert")).toHaveTextContent("First remote unavailable");
    view.rerender(
      <SkillDetailDialog
        target={{ variant: "market", skill: { name: "second", remotePath: "team/second" } }}
        t={t}
        onOpenChange={vi.fn()}
      />,
    );
    await screen.findByText("Skill instructions");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    view.rerender(
      <SkillDetailDialog target={{ variant: "installed", skill: { name: "local" } }} t={t} onOpenChange={vi.fn()} />,
    );
    await waitFor(() => expect(fetchSkillFile).toHaveBeenLastCalledWith("local/SKILL.md", undefined));
    await screen.findByText("Skill instructions");
  });

  it("ignores late completion for another repository with the same display name", async () => {
    let finish!: (file: SkillFile) => void;
    vi.mocked(fetchSkillFile).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const view = setup({ variant: "market", skill: { name: "demo", remotePath: "team-a/demo" } });
    view.rerender(
      <SkillDetailDialog
        target={{ variant: "market", skill: { name: "demo", remotePath: "team-b/demo" } }}
        t={t}
        onOpenChange={vi.fn()}
      />,
    );
    await screen.findByText("Skill instructions");
    await act(async () => finish({ path: "SKILL.md", content: "Wrong repository" }));
    expect(screen.queryByText("Wrong repository")).not.toBeInTheDocument();
  });

  it("allows retry after failure and resets the selected file after closing", async () => {
    const target = { variant: "installed" as const, skill: { name: "demo" } };
    vi.mocked(fetchSkillTree).mockResolvedValue({
      kind: "dir",
      path: "demo",
      entries: [{ name: "run.py", path: "demo/run.py", type: "file" }],
    });
    vi.mocked(fetchSkillFile).mockRejectedValueOnce(new Error("Temporary failure"));
    const view = setup(target);
    await screen.findByRole("alert");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "重试" }));
    await screen.findByText("Skill instructions");
    await user.click(screen.getByRole("button", { name: "run.py" }));
    await waitFor(() => expect(fetchSkillFile).toHaveBeenLastCalledWith("demo/run.py", undefined));
    view.rerender(<SkillDetailDialog target={null} t={t} onOpenChange={vi.fn()} />);
    view.rerender(<SkillDetailDialog target={target} t={t} onOpenChange={vi.fn()} />);
    await waitFor(() => expect(fetchSkillFile).toHaveBeenLastCalledWith("demo/SKILL.md", undefined));
  });
});
