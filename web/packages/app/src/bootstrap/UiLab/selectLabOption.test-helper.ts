import { screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";

export async function selectLabOption(user: UserEvent, label: string, option: string) {
  screen.getByRole("combobox", { name: label }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: option }));
}
