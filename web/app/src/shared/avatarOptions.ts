import { normalizeAvatarPath } from "@/shared/avatar";

export const AGENT_AVATAR_GROUPS = [
  { key: "3D", labelKey: "agentAvatarStyle3D" },
  { key: "cartoon", labelKey: "agentAvatarStyleCartoon" },
  { key: "pic", labelKey: "agentAvatarStylePic" },
] as const;

export const AGENT_AVATAR_OPTIONS = AGENT_AVATAR_GROUPS.flatMap((group) =>
  Array.from({ length: 8 }, (_, index) => ({
    group: group.key,
    labelKey: group.labelKey,
    index: index + 1,
    value: `avatar/${group.key}-${index + 1}.png`,
  })),
);

export type AvatarSource = {
  avatar?: string | null;
};

export function defaultAgentAvatar(): string {
  return AGENT_AVATAR_OPTIONS[0]?.value || "";
}

export function normalizeAgentAvatarPath(value: unknown): string {
  return normalizeAvatarPath(value);
}

export function selectUnusedAgentAvatar(sources: readonly AvatarSource[] | null | undefined): string {
  const used = new Set(
    (sources ?? [])
      .map((source) => normalizeAgentAvatarPath(source?.avatar))
      .filter((avatar): avatar is string => Boolean(avatar)),
  );
  const available = AGENT_AVATAR_OPTIONS.filter((option) => !used.has(option.value));
  const candidates = available.length > 0 ? available : AGENT_AVATAR_OPTIONS;
  return candidates[randomIndex(candidates.length)]?.value || defaultAgentAvatar();
}

function randomIndex(length: number): number {
  if (length <= 1) {
    return 0;
  }
  if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
    const bytes = new Uint32Array(1);
    crypto.getRandomValues(bytes);
    return bytes[0] % length;
  }
  return Math.floor(Math.random() * length);
}

/** Deterministic avatar selection based on a seed string (e.g. agent ID).
 *  Produces a stable per-seed choice so the same agent always renders the same avatar,
 *  while distributing different agents across the full option set. */
export function deterministicAvatarForSeed(seed: string): string {
  if (!seed) {
    return defaultAgentAvatar();
  }
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = ((hash << 5) - hash + seed.charCodeAt(i)) | 0;
  }
  const index = ((hash >>> 0) % AGENT_AVATAR_OPTIONS.length) || 0;
  return AGENT_AVATAR_OPTIONS[index]?.value || defaultAgentAvatar();
}
