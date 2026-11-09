const AVATAR_BASE = "avatar";
const MANAGER_AVATAR = `${AVATAR_BASE}/manager.png`;
export const TEMPLATE_AVATAR = `${AVATAR_BASE}/template.png`;
export const CLOUD_AGENT_AVATARS = {
  openclaw: `${AVATAR_BASE}/cloud-openclaw.png`,
  langflow: `${AVATAR_BASE}/cloud-langflow.png`,
  csgclaw: `${AVATAR_BASE}/cloud-csgclaw.png`,
  code: `${AVATAR_BASE}/cloud-code.png`,
} as const;

const AVATAR_GROUPS = ["3D", "cartoon", "pic"] as const;

const AVATAR_VALUE_SET = new Set([
  MANAGER_AVATAR,
  TEMPLATE_AVATAR,
  ...Object.values(CLOUD_AGENT_AVATARS),
  ...AVATAR_GROUPS.flatMap((group) =>
    Array.from({ length: 8 }, (_, index) => `${AVATAR_BASE}/${group}-${index + 1}.png`),
  ),
]);

export function normalizeAvatarPath(value: unknown): string {
  const avatar = String(value ?? "").trim();
  if (AVATAR_VALUE_SET.has(avatar)) {
    return avatar;
  }
  if (/^(https?:\/\/|\/|data:)/i.test(avatar)) {
    return avatar;
  }
  return "";
}

export function avatarFallbackText(...labels: Array<string | null | undefined>): string {
  for (const label of labels) {
    const text = String(label ?? "").trim();
    if (text) {
      const firstCharacter = Array.from(text)[0] || "";
      return firstCharacter.toUpperCase();
    }
  }
  return "#";
}
