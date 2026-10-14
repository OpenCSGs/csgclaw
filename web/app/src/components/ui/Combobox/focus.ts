export function nextFocusTarget(
  trigger: HTMLElement,
  popup: HTMLElement | null,
  backwards: boolean,
): HTMLElement | null {
  const dialog = trigger.closest<HTMLElement>('[role="dialog"]');
  const scope = dialog ?? document;
  const controls = Array.from(
    scope.querySelectorAll<HTMLElement>(
      'a[href], button, input, select, textarea, [tabindex], [contenteditable="true"]',
    ),
  ).filter((element) => {
    if (element === trigger) return true;
    if (element.tabIndex < 0 || element.matches(":disabled") || popup?.contains(element)) return false;
    if (element.closest("[hidden], [inert]")) return false;
    const hidden = element.closest('[aria-hidden="true"]');
    if (hidden && !(dialog && hidden.contains(dialog))) return false;
    const style = window.getComputedStyle(element);
    return style.display !== "none" && style.visibility !== "hidden";
  });
  const index = controls.indexOf(trigger);
  const targetIndex = index + (backwards ? -1 : 1);
  if (index < 0) return null;
  if (dialog && controls.length) return controls[(targetIndex + controls.length) % controls.length];
  return controls[targetIndex] ?? null;
}
