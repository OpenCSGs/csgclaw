export const OPEN_ONBOARDING_GUIDE_EVENT = "csgclaw:open-guide";

export function openOnboardingGuide() {
  window.dispatchEvent(new Event(OPEN_ONBOARDING_GUIDE_EVENT));
}
