import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TeamDetailPane } from "./TeamDetailPane";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTeam } from "@/models/tasks";

const t: TranslateFn = (key) => key;

const team: WorkspaceTeam = {
  id: "team-1",
  title: "Windows team",
  lead_agent_id: "u-manager",
  member_agent_ids: [],
  status: "active",
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-01T00:00:00Z",
};

describe("TeamDetailPane", () => {
  it("places the collaboration guide between members and parent tasks", () => {
    render(<TeamDetailPane team={team} t={t} />);
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "teamMembersTab",
      "teamGuideTab",
      "teamRecordsTab",
    ]);
  });

  it("grows the guide editor with content and shrinks it after clearing", async () => {
    const user = userEvent.setup();
    render(<TeamDetailPane team={team} t={t} onUpdateGuide={vi.fn().mockResolvedValue(true)} />);
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    const editor = screen.getByRole("textbox", { name: "teamGuideTab" }) as HTMLTextAreaElement;
    let contentHeight = 130;
    Object.defineProperty(editor, "scrollHeight", { configurable: true, get: () => contentHeight });
    fireEvent.change(editor, { target: { value: "Short guide" } });
    const compactHeight = parseFloat(editor.style.height);
    contentHeight = 380;
    fireEvent.change(editor, { target: { value: "A longer guide\n".repeat(15) } });
    expect(parseFloat(editor.style.height)).toBeGreaterThan(compactHeight);
    contentHeight = 130;
    await user.clear(editor);
    expect(parseFloat(editor.style.height)).toBe(compactHeight);
  });

  it("edits a separate collaboration guide and saves only on confirmation", async () => {
    const user = userEvent.setup();
    const onUpdateGuide = vi.fn().mockResolvedValue(true);
    const view = render(<TeamDetailPane team={team} t={t} onUpdateGuide={onUpdateGuide} />);
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    expect(screen.getByRole("status")).toHaveTextContent("teamGuideSaved");
    expect(screen.getByRole("button", { name: "save" })).toBeDisabled();
    await user.type(screen.getByRole("textbox", { name: "teamGuideTab" }), "Review docs/ before handing off.");
    expect(screen.getByRole("status")).toHaveTextContent("teamGuideDirty");
    expect(onUpdateGuide).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "save" }));
    expect(onUpdateGuide).toHaveBeenCalledWith(team, "Review docs/ before handing off.");
    view.rerender(
      <TeamDetailPane
        team={{ ...team, collaboration_guide: "Review docs/ before handing off." }}
        t={t}
        onUpdateGuide={onUpdateGuide}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("teamGuideSaved");
    expect(screen.getByRole("button", { name: "save" })).toBeDisabled();
  });

  it("mentions only this team's members and inserts a selected name as plain text", async () => {
    const user = userEvent.setup();
    const onUpdateGuide = vi.fn().mockResolvedValue(true);
    render(
      <TeamDetailPane
        team={{ ...team, member_agent_ids: ["agent-dev"] }}
        agents={[
          { id: "u-manager", name: "Manager", role: "manager" },
          { id: "agent-dev", name: "开发工程师 Herry", role: "worker" },
          { id: "agent-outsider", name: "Other team member", role: "worker" },
        ]}
        t={t}
        onUpdateGuide={onUpdateGuide}
      />,
    );
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    const editor = screen.getByRole("textbox", { name: "teamGuideTab" });
    await user.type(editor, "Ask @");
    expect(screen.getAllByRole("option")).toHaveLength(2);
    expect(screen.queryByRole("option", { name: /Other team member/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: /@开发工程师 Herry/ }));
    expect(editor).toHaveValue("Ask @开发工程师 Herry ");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(onUpdateGuide).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "save" }));
    expect(onUpdateGuide).toHaveBeenCalledWith(expect.anything(), "Ask @开发工程师 Herry ");
  });

  it("marks a whitespace-only guide as saved after the server normalizes it", async () => {
    const user = userEvent.setup();
    const onUpdateGuide = vi.fn().mockResolvedValue(true);
    render(<TeamDetailPane team={team} t={t} onUpdateGuide={onUpdateGuide} />);
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    const editor = screen.getByRole("textbox", { name: "teamGuideTab" });
    await user.type(editor, "   ");
    await user.click(screen.getByRole("button", { name: "save" }));
    expect(onUpdateGuide).toHaveBeenCalledWith(team, "   ");
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("teamGuideSaved"));
    expect(editor).toHaveValue("");
  });

  it("filters mentions, supports keyboard selection, and ignores IME confirmation", async () => {
    const user = userEvent.setup();
    render(
      <TeamDetailPane
        team={{ ...team, member_agent_ids: ["agent-dev"] }}
        agents={[
          { id: "u-manager", name: "Manager", role: "manager" },
          { id: "agent-dev", name: "Developer", role: "worker" },
        ]}
        t={t}
        onUpdateGuide={vi.fn().mockResolvedValue(true)}
      />,
    );
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    const editor = screen.getByRole("textbox", { name: "teamGuideTab" });
    await user.type(editor, "@");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(editor).toHaveValue("@Manager ");
    await user.clear(editor);
    fireEvent.compositionStart(editor);
    fireEvent.change(editor, { target: { value: "@De" } });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.compositionEnd(editor);
    expect(screen.getAllByRole("option")).toHaveLength(1);
    fireEvent.keyDown(editor, { key: "Enter", isComposing: true });
    expect(editor).toHaveValue("@De");
    await user.keyboard("{Tab}");
    expect(editor).toHaveValue("@Developer ");
    await user.type(editor, "@");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("retains a failed save and prevents duplicate saves while a request is pending", async () => {
    const user = userEvent.setup();
    let finish!: (saved: boolean) => void;
    const onUpdateGuide = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    render(<TeamDetailPane team={team} t={t} onUpdateGuide={onUpdateGuide} />);
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    const editor = screen.getByRole("textbox", { name: "teamGuideTab" });
    await user.type(editor, "Keep this draft");
    await user.click(screen.getByRole("button", { name: "save" }));
    expect(editor).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("teamSaving");
    expect(onUpdateGuide).toHaveBeenCalledTimes(1);
    finish(false);
    await waitFor(() => expect(editor).toBeEnabled());
    expect(editor).toHaveValue("Keep this draft");
    expect(screen.getByRole("status")).toHaveTextContent("teamGuideDirty");
    expect(screen.getByRole("alert")).toHaveTextContent("teamActionFailed");
    expect(screen.getByRole("button", { name: "save" })).toBeEnabled();
  });

  it("keeps guide drafts when changing tabs and resets them when selecting another team", async () => {
    const user = userEvent.setup();
    const props = { t, onUpdateGuide: vi.fn().mockResolvedValue(true) };
    const view = render(<TeamDetailPane {...props} team={team} />);
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    await user.type(screen.getByRole("textbox", { name: "teamGuideTab" }), "Unfinished draft");
    await user.click(screen.getByRole("tab", { name: "teamMembersTab" }));
    await user.click(screen.getByRole("tab", { name: "teamGuideTab" }));
    expect(screen.getByRole("textbox", { name: "teamGuideTab" })).toHaveValue("Unfinished draft");
    view.rerender(
      <TeamDetailPane {...props} team={{ ...team, id: "team-other", collaboration_guide: "Other rules" }} />,
    );
    expect(screen.getByRole("textbox", { name: "teamGuideTab" })).toHaveValue("Other rules");
    expect(screen.getByRole("status")).toHaveTextContent("teamGuideSaved");
  });
  it("saves and clears descriptions on blur and cancels with Escape", async () => {
    const user = userEvent.setup();
    const onUpdateDescription = vi.fn().mockResolvedValue(true);
    render(<TeamDetailPane team={team} t={t} onUpdateDescription={onUpdateDescription} />);
    await user.click(screen.getByRole("button", { name: "editDescription" }));
    const input = screen.getByRole("textbox", { name: "teamDescriptionLabel" });
    await user.type(input, "Team introduction");
    await user.tab();
    expect(onUpdateDescription).toHaveBeenCalledWith(team, "Team introduction");
    await user.click(screen.getByRole("button", { name: "editDescription" }));
    await user.clear(screen.getByRole("textbox", { name: "teamDescriptionLabel" }));
    await user.keyboard("{Escape}");
    expect(onUpdateDescription).toHaveBeenCalledTimes(1);
  });
  it("shows only the saved team description and leaves missing descriptions blank", () => {
    const view = render(<TeamDetailPane team={{ ...team, description: "Review releases together." }} t={t} />);
    expect(screen.getByText("Review releases together.")).toBeVisible();

    view.rerender(<TeamDetailPane team={{ ...team, description: "  " }} t={t} />);
    expect(screen.queryByText("Review releases together.")).not.toBeInTheDocument();
    expect(screen.queryByText("teamDetailSubtitle")).not.toBeInTheDocument();

    view.rerender(<TeamDetailPane team={team} t={t} />);
    expect(screen.queryByText("teamDetailSubtitle")).not.toBeInTheDocument();
  });
  it("starts collaboration with the current team", async () => {
    const onStartCollaboration = vi.fn();
    render(<TeamDetailPane team={team} t={t} onStartCollaboration={onStartCollaboration} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "teamStartCollaboration" }));
    expect(onStartCollaboration).toHaveBeenCalledExactlyOnceWith(team);
  });
  it("confirms team deletion in-app without opening a native browser dialog", async () => {
    const user = userEvent.setup();
    const onDeleteTeam = vi.fn().mockResolvedValue(true);
    const confirmSpy = vi.spyOn(window, "confirm");

    try {
      render(<TeamDetailPane team={team} t={t} onDeleteTeam={onDeleteTeam} />);

      await user.click(screen.getByRole("button", { name: "teamDelete" }));

      const dialog = screen.getByRole("dialog", { name: "teamDelete" });
      expect(within(dialog).getByText("teamDeleteConfirm")).toBeInTheDocument();
      expect(onDeleteTeam).not.toHaveBeenCalled();
      expect(confirmSpy).not.toHaveBeenCalled();

      await user.click(within(dialog).getByRole("button", { name: "teamDelete" }));

      await waitFor(() => expect(onDeleteTeam).toHaveBeenCalledWith(team));
      expect(confirmSpy).not.toHaveBeenCalled();
    } finally {
      confirmSpy.mockRestore();
    }
  });
});
