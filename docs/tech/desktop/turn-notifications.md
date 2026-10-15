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

macOS, Windows, and Linux use the same Electron main-process notification API.
The permission indicator and request action use the renderer's standard Notification API on every platform.
Electron's permission status does not report operating-system banner, sound, or Focus settings.
Clicking a notification opens its conversation in CSGClaw.

On macOS 27.0.1, temporary banners can fail to appear while CSGClaw is focused even when notification delivery is accepted.
This foreground presentation issue remains unresolved.
