import { AgentIcon } from "@/components/ui/Icons";
import type { ReactNode } from "react";

const AVATAR_BASE = "avatar";

const AVATAR_GROUPS = [
  { key: "3D", labelKey: "agentAvatarStyle3D" },
  { key: "cartoon", labelKey: "agentAvatarStyleCartoon" },
  { key: "pic", labelKey: "agentAvatarStylePic" },
] as const;

export const AGENT_AVATAR_OPTIONS = AVATAR_GROUPS.flatMap((group) =>
  Array.from({ length: 8 }, (_, index) => ({
    group: group.key,
    labelKey: group.labelKey,
    index: index + 1,
    value: `${AVATAR_BASE}/${group.key}-${index + 1}.png`,
  })),
);

const AGENT_AVATAR_VALUE_SET = new Set(AGENT_AVATAR_OPTIONS.map((option) => option.value));

type TranslateFn = (key: string) => string;

export function defaultAgentAvatar(): string {
  return AGENT_AVATAR_OPTIONS[0]?.value || "";
}

export function avatarFallbackText(...labels: Array<string | null | undefined>): string {
  for (const label of labels) {
    const text = String(label ?? "").trim();
    if (text) {
      return text.slice(0, 2).toUpperCase();
    }
  }
  return "#";
}

export function normalizeAgentAvatarPath(value: unknown): string {
  const avatar = String(value ?? "").trim();
  if (AGENT_AVATAR_VALUE_SET.has(avatar)) {
    return avatar;
  }
  if (/^(https?:\/\/|\/|data:)/i.test(avatar)) {
    return avatar;
  }
  return "";
}

export function AgentAvatarImage({ avatar, alt = "" }: { avatar?: string | null; alt?: string }) {
  const src = normalizeAgentAvatarPath(avatar);
  if (!src) {
    return <AgentIcon />;
  }
  return <img className="agent-avatar-image" src={src} alt={alt} draggable={false} />;
}

export function AgentAvatarContent({
  avatar,
  fallback,
  alt = "",
}: {
  avatar?: string | null;
  fallback?: ReactNode;
  alt?: string;
}) {
  const src = normalizeAgentAvatarPath(avatar);
  if (!src) {
    return <span className="agent-avatar-fallback">{fallback ?? avatar}</span>;
  }
  return <img className="agent-avatar-image" src={src} alt={alt} draggable={false} />;
}

export function AgentAvatarPicker({
  value,
  t,
  onChange,
}: {
  value?: string | null;
  t: TranslateFn;
  onChange: (value: string) => void;
}) {
  const selected = normalizeAgentAvatarPath(value) || defaultAgentAvatar();
  return (
    <div className="agent-avatar-picker" role="radiogroup" aria-label={t("agentAvatar")}>
      {AVATAR_GROUPS.map((group) => (
        <div className="agent-avatar-picker-group" key={group.key}>
          <div className="agent-avatar-picker-label">{t(group.labelKey)}</div>
          <div className="agent-avatar-picker-options">
            {AGENT_AVATAR_OPTIONS.filter((option) => option.group === group.key).map((option) => {
              const checked = option.value === selected;
              const label = `${t(option.labelKey)} ${option.index}`;
              return (
                <button
                  aria-checked={checked}
                  aria-label={label}
                  className={`agent-avatar-option ${checked ? "selected" : ""}`}
                  key={option.value}
                  role="radio"
                  title={label}
                  type="button"
                  onClick={() => onChange(option.value)}
                >
                  <img src={option.value} alt="" draggable={false} />
                </button>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
