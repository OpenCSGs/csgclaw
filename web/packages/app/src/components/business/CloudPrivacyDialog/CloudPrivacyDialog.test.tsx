import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import { CloudPrivacyDialog } from "./CloudPrivacyDialog";

it.each([
  ["zh", "继续"],
  ["en", "Continue"],
] as const)("keeps the %s confirmation accessible with a concise action", async (locale, label) => {
  const t = createTranslator(locale);
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const user = userEvent.setup();
  render(<CloudPrivacyDialog agentName="Cloud agent" confirmation={{ onConfirm, onCancel }} t={t} />);
  expect(screen.getByRole("dialog", { name: t("cloudPrivacyTitle") })).toHaveAccessibleDescription(
    t("cloudPrivacyMessage"),
  );
  expect(screen.getByRole("checkbox", { name: t("cloudPrivacyRemember") })).not.toBeChecked();
  await user.click(screen.getByRole("checkbox", { name: t("cloudPrivacyRemember") }));
  await user.click(screen.getByRole("button", { name: label }));
  expect(onConfirm).toHaveBeenCalledWith(true);
  expect(onCancel).not.toHaveBeenCalled();
});
