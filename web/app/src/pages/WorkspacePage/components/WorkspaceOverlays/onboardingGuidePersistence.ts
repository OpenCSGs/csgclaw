// Advance only when guide content warrants another automatic presentation.
export const GUIDE_REVISION = "1";
export const GUIDE_ACKNOWLEDGED_KEY = "csgclaw:onboarding-guide:acknowledged-revision";
const LEGACY_SEEN_PREFIX = "csgclaw:onboarding-guide:seen:";
const LEGACY_GUIDE_REVISION = "1";

export function hasAcknowledgedGuide(storage: Storage, revision: string = GUIDE_REVISION): boolean {
  const acknowledged = storage.getItem(GUIDE_ACKNOWLEDGED_KEY);
  if (acknowledged !== null) return acknowledged === revision;

  // App-version records represent the original content only. Never migrate
  // them to a later revision, or they would suppress genuinely new content.
  if (revision !== LEGACY_GUIDE_REVISION) return false;
  for (let index = 0; index < storage.length; index++) {
    const key = storage.key(index);
    if (key?.startsWith(LEGACY_SEEN_PREFIX) && storage.getItem(key) === "seen") {
      try {
        storage.setItem(GUIDE_ACKNOWLEDGED_KEY, revision);
      } catch {
        // Honor the existing record even when migration cannot be persisted.
      }
      return true;
    }
  }
  return false;
}
