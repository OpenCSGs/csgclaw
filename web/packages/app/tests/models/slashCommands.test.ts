import {
  composerActionSuggestions,
  isNewConversationSlashCommand,
  parseSlashCommand,
  renderSlashCommandPreviewText,
} from "@/models/slashCommands";

describe("slash command parser", () => {
  it("rejects duplicate slash command attributes", () => {
    expect(
      parseSlashCommand('<slash-command name="use-skill" name="use-skill" arg="skill-creator"></slash-command> create'),
    ).toBeNull();
  });

  it("renders canonical slash command as preview text", () => {
    expect(
      renderSlashCommandPreviewText(
        '<slash-command name="use-skill" arg="skill-creator"></slash-command> build a skill',
      ),
    ).toBe("/skill-creator build a skill");
    expect(
      renderSlashCommandPreviewText('<slash-command name="new" arg="conversation"></slash-command> reset first'),
    ).toBe("/new reset first");
  });

  it("identifies only the supported new-conversation command", () => {
    expect(isNewConversationSlashCommand('<slash-command name="new" arg="conversation"></slash-command>')).toBe(true);
    expect(isNewConversationSlashCommand('<slash-command name="use-skill" arg="new"></slash-command>')).toBe(false);
    expect(isNewConversationSlashCommand("/new")).toBe(false);
  });

  it("suggests creation commands without executing natural-language actions", () => {
    expect(composerActionSuggestions("帮我创建一个 dev 智能体")).toEqual([
      expect.objectContaining({ name: "创建智能体", type: "command" }),
    ]);
    expect(composerActionSuggestions("新建一个项目房间")).toEqual([
      expect.objectContaining({ name: "创建房间", type: "command" }),
    ]);
    expect(composerActionSuggestions("/创建房间")).toEqual([]);
  });
});
