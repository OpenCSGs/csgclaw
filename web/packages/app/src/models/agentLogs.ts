// Only rewrite a logger's timestamp field/prefix, never dates in its message.
const logTimestamp =
  /^(\s*(?:\{\s*"(?:timestamp|time)"\s*:\s*"|time="?))?(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})(?=["\s]|$)/;

export function localizeAgentLogTimestamps(content: string): string {
  return content.replace(/^.*$/gm, (line) =>
    line.replace(
      logTimestamp,
      (original, prefix: string | undefined, seconds: string, fraction: string | undefined, zone: string) => {
        const date = new Date(`${seconds}${zone}`);
        if (Number.isNaN(date.getTime())) return original;
        const pad = (value: number) => String(value).padStart(2, "0");
        const offset = -date.getTimezoneOffset();
        const local = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
        const localZone = `${offset >= 0 ? "+" : "-"}${pad(Math.floor(Math.abs(offset) / 60))}:${pad(Math.abs(offset) % 60)}`;
        return `${prefix || ""}${local}${fraction || ""}${localZone}`;
      },
    ),
  );
}
