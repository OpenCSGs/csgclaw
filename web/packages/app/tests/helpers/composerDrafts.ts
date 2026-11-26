import { saveComposerDraft } from "@/shared/storage/composerDrafts";
import { COMPOSER_DRAFT_STORAGE_PREFIX } from "@/shared/storage/keys";

export function clearTestComposerDrafts() {
  window.dispatchEvent(new Event("pagehide"));
  for (const key of Object.keys(localStorage)) {
    if (key.startsWith(COMPOSER_DRAFT_STORAGE_PREFIX)) saveComposerDraft(key, null, []);
  }
}
