import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createTranslator } from "@/shared/i18n";
import { TooltipProvider } from "@/components/ui";
import buttonStyles from "@/components/ui/Button/Button.module.css";
import { ConversationSection } from "./ConversationSection";

test("conversation deletion uses the scoped danger button and keeps selection separate", async () => {
  const user = userEvent.setup();
  const onSelect = vi.fn();
  const onDelete = vi.fn();
  const t = createTranslator("en");
  render(
    <TooltipProvider>
      <ConversationSection
        title="Conversations"
        items={[{ id: "room-1", title: "Project room", members: [], messages: [] }]}
        activeConversationId="room-1"
        currentUserID="user-1"
        usersById={new Map()}
        locale="en"
        t={t}
        onSelect={onSelect}
        onDelete={onDelete}
      />
    </TooltipProvider>,
  );
  const remove = screen.getByRole("button", { name: `${t("deleteRoom")} Project room` });
  expect(remove).toHaveClass(buttonStyles.btn, buttonStyles["btn-outline-danger"], buttonStyles["btn-sm"]);
  expect(remove).toHaveAttribute("type", "button");
  await user.tab();
  await user.tab();
  expect(remove).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(onDelete).toHaveBeenCalledWith("room-1");
  expect(onSelect).not.toHaveBeenCalled();
});
