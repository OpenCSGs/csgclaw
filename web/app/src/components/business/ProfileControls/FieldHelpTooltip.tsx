import profileControlsStyles from "./ProfileControls.module.css";
import { Tooltip } from "@/components/ui/Tooltip";

export type FieldHelpTooltipProps = {
  detail?: string;
  summary?: string;
};

export function FieldHelpTooltip({ detail, summary }: FieldHelpTooltipProps) {
  const body = [summary, detail]
    .map((value) => String(value ?? "").trim())
    .filter(Boolean)
    .join("\n\n");
  if (!body) return null;
  return (
    <Tooltip content={body} contentProps={{ side: "right", className: profileControlsStyles["field-help-flyout"] }}>
      <button type="button" className={profileControlsStyles["field-help-trigger"]} aria-label={summary || body}>
        ?
      </button>
    </Tooltip>
  );
}
