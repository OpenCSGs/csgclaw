# Windows theme icons

The desktop window uses the selected light/dark icon, or follows the system when
the selected theme is `system`. The effective icon is cached before window
creation and restored when the window is recreated or shown from the tray.
Repeated notifications for the same effective icon are ignored. Application
theme changes update the tray before preference persistence and reapply the
latest tray icon after a short 16 ms settle window. Rapid preference changes are
persisted once after a separate 150 ms debounce, outside the icon update path.
The taskbar button is removed on a fixed 100 ms deadline, then restored with
the latest icon after a separate 75 ms Explorer processing window. Superseded
refreshes cannot restore the button early with an intermediate theme.

Live window icon updates use the bundled icon immediately. Shortcut enumeration,
icon persistence and shortcut writes run only for the final selection in the
scheduled taskbar refresh. Rapid intermediate themes no longer perform these
synchronous disk operations before updating the live icon. New clicks update the
selected icon without moving the deadline, so continuous input cannot starve
refresh. Each active 100 ms window processes the latest selection, including
changes during the 75 ms removal wait. No refresh repeats while idle. These are
timer targets, not display latency guarantees. Button reconstruction can still
cause visible movement; native Windows testing is required to assess smoothness.

For installed Squirrel builds, Explorer can prefer the shortcut icon for the
application's taskbar group over `BrowserWindow.setIcon()`. Theme changes update
the **entire shortcut icon**, not an overlay, in the current user's standard
Start Menu, desktop and pinned-taskbar locations. Only links matching both the
CSGClaw AppUserModelID and this installation's stable launcher are eligible.
Custom links, other installations and version-directory launch targets are left
alone. Missing or unwritable links are logged without preventing window updates.

The update changes only `icon` and `iconIndex` using Electron's `update`
operation. It preserves the shortcut's target, arguments, working directory,
AppUserModelID and other metadata. Electron delegates to Chromium's shortcut
writer, which notifies Explorer with `SHChangeNotify(SHCNE_UPDATEITEM)`.
Content-addressed ICO files under the desktop user-data `taskbar-icons` directory
remain available after Squirrel removes old `app-<version>` directories.

The AppUserModelID stays `com.squirrel.csgclaw_desktop.CSGClaw` for every theme.
No theme-specific group, custom relaunch command, repinning, Explorer restart or
global icon-cache reset is used. MSIX builds update only the live window icon;
their manifest continues to own package identity and the pinned icon. Development
and unpackaged builds do not modify installed shortcuts.

## Verification

From `desktop/`:

```powershell
pnpm run build
node --test dist/main/windowsTaskbar.test.js dist/main/windowsShortcuts.test.js dist/shared/desktopTheme.test.js
pnpm exec electron scripts/test-windows-shortcuts.cjs
```

The native Windows test creates shortcuts only in a temporary fixture. It checks
real light/dark/light ICO updates, all retained launch fields, and icon persistence
across two simulated version-directory changes. It does not prove Explorer's
visible taskbar rendering or exercise actual Squirrel upgrades.

Before release, verify an installed Squirrel build with and without an existing
pin: switch light/dark/system, hide to the tray and restore, then launch the same
pin after two real updates. Confirm one taskbar group, the full themed icon, and
the latest app version. Check MSIX separately for unchanged package identity.
Explorer may defer a pinned icon refresh on some Windows versions; validate the
visible result on supported versions rather than relying on shortcut metadata
tests alone.

## References

- [PR #546 review](https://github.com/OpenCSGs/csgclaw/pull/546#pullrequestreview-5100603040)
- [Electron shortcut API](https://www.electronjs.org/docs/latest/api/shell#shellwriteshortcutlinkshortcutpath-operation-options-windows)
- [Chromium shortcut writer](https://github.com/chromium/chromium/blob/main/base/win/shortcut.cc)
- [Windows taskbar relaunch icon](https://learn.microsoft.com/en-us/windows/win32/properties/props-system-appusermodel-relaunchiconresource)
