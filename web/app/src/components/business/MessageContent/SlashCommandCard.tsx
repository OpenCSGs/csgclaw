import type { SlashCommandPayload } from "./slashCommands";

type SlashCommandCardProps = {
  command: SlashCommandPayload;
};

export function SlashCommandCard({ command }: SlashCommandCardProps) {
  return (
    <div className="slash-command-card" aria-label="Slash command">
      <div className="slash-command-header">
        <span className="slash-command-kicker">Slash command</span>
        <code className="slash-command-name">{command.name}</code>
        {command.arg ? <code className="slash-command-arg">{command.arg}</code> : null}
      </div>
      {command.body ? <div className="slash-command-body">{command.body}</div> : null}
    </div>
  );
}
