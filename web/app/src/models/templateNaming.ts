export const TEMPLATE_NAME_MAX_LENGTH = 32;

// Milliseconds encoded in base 36 keep the timestamp short without losing precision.
export function defaultTemplateName(agentID: string, timestamp = Date.now()): string {
  const suffix = `-${timestamp.toString(36)}`;
  let prefix = agentID.trim().replace(/[^A-Za-z0-9_-]/g, "-");
  if (!/^[A-Za-z]/.test(prefix)) prefix = `agent-${prefix}`;
  return `${prefix.slice(0, TEMPLATE_NAME_MAX_LENGTH - suffix.length)}${suffix}`;
}

export function isValidTemplateName(name: string): boolean {
  return /^[A-Za-z][A-Za-z0-9_-]{0,31}$/.test(name.trim());
}
