import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import { NotificationsSection } from "@/pages/SettingsPage/sections/NotificationsSection";
import { createTranslator } from "@/shared/i18n";
import { readStoredTurnNotificationSound, writeStoredTurnNotificationSound } from "@/shared/storage/turnNotifications";
import { TURN_NOTIFICATION_SOUND_STORAGE_KEY } from "@/shared/storage/keys";

describe("notification sounds", () => {
  afterEach(() => localStorage.removeItem(TURN_NOTIFICATION_SOUND_STORAGE_KEY));

  it("defaults to audible notifications and remembers a muted preference", () => {
    expect(readStoredTurnNotificationSound()).toBe(true);
    writeStoredTurnNotificationSound(false);
    expect(readStoredTurnNotificationSound()).toBe(false);
    writeStoredTurnNotificationSound(true);
    expect(readStoredTurnNotificationSound()).toBe(true);
  });

  it("lets users disable sound independently of the notification mode", async () => {
    const onTurnNotificationSoundChange = vi.fn();
    render(
      <WorkspaceControllerProvider
        controller={
          {
            ready: true,
            sidebarProps: {
              t: createTranslator("zh"),
              turnNotificationMode: "always",
              turnNotificationPermission: "granted",
              turnNotificationSound: true,
              onTurnNotificationSoundChange,
            },
          } as never
        }
      >
        <NotificationsSection />
      </WorkspaceControllerProvider>,
    );
    const control = screen.getByRole("switch", { name: "播放通知声音" });
    expect(control).toBeChecked();
    await userEvent.setup().click(control);
    expect(onTurnNotificationSoundChange).toHaveBeenCalledWith(false);
    expect(screen.getByText("始终通知")).toBeInTheDocument();
  });
});
