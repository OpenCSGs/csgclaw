# Turn completion notifications

Settings > Notifications controls system notifications for completed agent turns.
The default mode notifies only while the app is unfocused.
Always notify also sends notifications while CSGClaw is focused.
Off suppresses completion notifications entirely.

Play notification sounds defaults to enabled and is independent of the notification mode.
Disabling it preserves the notification banner and sets the native or browser notification's `silent` option.
The preference is stored locally and retained across reloads and restarts.
System notification permissions, banner style, and Focus settings remain controlled by the operating system.

Completion notifications use terminal turn progress or explicit final replies, never running progress, commentary, or agent-authored ordinary messages.
A turn's final reply and work-release event share a notification identity scoped to its room and agent.
The Electron main process retains native notification objects until dismissal, activation, or bounded history eviction so macOS does not remove them during garbage collection.

On macOS, a bundled accessory application named CSGClaw Notifications delivers notifications independently of the foreground window.
This avoids foreground suppression observed when Electron delivers notifications under the main application's identity.
The helper uses the normal macOS notification service and respects Focus and screen-sharing notification rules.
Allow notifications for “CSGClaw 通知” when macOS asks; the Settings permission indicator reads this helper's actual authorization.
The sound preference controls the helper's notification sound independently of the banner.
Clicking a notification forwards its room identity to the main application.
The helper exits when the desktop process closes its input stream.

The desktop build compiles and signs the helper on macOS and packages it as an extra resource outside `app.asar`.
The target architecture follows `CSGCLAW_DESKTOP_ARCH` or `CSGCLAW_DESKTOP_GOARCH`.
Windows and Linux continue using Electron's notification API.
