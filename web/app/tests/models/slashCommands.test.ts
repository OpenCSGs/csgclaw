import { parseSlashCommand, renderSlashCommandPreviewText } from "@/models/slashCommands";

describe("slash command parser", () => {
  it("rejects duplicate slash command attributes", () => {
    expect(
      parseSlashCommand(
        '<slash-command name="use-skill" name="use-skill" arg="skill-creator"></slash-command> create',
      ),
    ).toBeNull();
  });

  it("renders canonical slash command as preview text", () => {
    expect(
      renderSlashCommandPreviewText(
        '<slash-command name="use-skill" arg="skill-creator"></slash-command> build a skill',
      ),
    ).toBe("/skill-creator build a skill");
  });
});
