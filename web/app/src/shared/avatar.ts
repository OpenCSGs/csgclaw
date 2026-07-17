const AVATAR_BASE = "avatar";
const MANAGER_AVATAR = `${AVATAR_BASE}/manager.png`;

const AVATAR_GROUPS = ["3D", "cartoon", "pic"] as const;

const AVATAR_VALUE_SET = new Set(
  [
    MANAGER_AVATAR,
    ...AVATAR_GROUPS.flatMap((group) =>
      Array.from({ length: 8 }, (_, index) => `${AVATAR_BASE}/${group}-${index + 1}.png`),
    ),
  ],
);

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
