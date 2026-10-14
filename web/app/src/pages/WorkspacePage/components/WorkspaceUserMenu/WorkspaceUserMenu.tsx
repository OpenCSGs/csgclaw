import { useState } from "react";
import { Check, ChevronUp, LogOut, Monitor, Moon, Sun, User, Settings as SettingsIcon } from "lucide-react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import { avatarFallbackText } from "@/shared/avatar";
import { classNames } from "@/shared/lib/classNames";
import styles from "./WorkspaceUserMenu.module.css";
import type { IMUser, LocaleCode, TranslateFn } from "@/models/conversations";
import type { ThemeMode } from "@/shared/theme/theme";

export type WorkspaceUserMenuProps = {
  currentUser: IMUser | undefined;

  openCSG: {
    authenticated: boolean;
    expired?: boolean;

    baseURL?: string;
  };
  locale: LocaleCode;

  presentation?: "icon" | "row";
  theme: ThemeMode;
  t: TranslateFn;
  onLocaleChange: (locale: LocaleCode) => void;

  onOpenCSGLogin: () => void;

  onOpenCSGLogout: () => void;
  onOpenProfile: () => void;
  onOpenSettings: () => void;
  onThemeChange: (theme: ThemeMode) => void;
};

const THEME_OPTIONS = [
  { icon: Monitor, key: "system" as const, labelKey: "themeSystem" as const },
  { icon: Sun, key: "light" as const, labelKey: "themeLight" as const },
  { icon: Moon, key: "dark" as const, labelKey: "themeDark" as const },
];

export function WorkspaceUserMenu({
  currentUser,
  openCSG,
  locale,
  presentation = "row",
  theme,
  t,
  onLocaleChange,
  onOpenCSGLogin,
  onOpenCSGLogout,
  onOpenProfile,
  onOpenSettings,
  onThemeChange,
}: WorkspaceUserMenuProps) {
  const [open, setOpen] = useState(false);
  const name = currentUser?.name || currentUser?.id || t("localIdentityFallback");
  const role = currentUser?.role || "";
  const iconOnly = presentation === "icon";

  return (
    <DropdownMenuRoot open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={classNames(styles.trigger, iconOnly && styles.triggerIconOnly, open && styles.triggerOpen)}
          aria-label={t("iaUserMenuLabel")}
        >
          <span className={classNames("iaAvatarFrame", styles.avatar)} aria-hidden="true">
            <AgentAvatarContent
              avatar={currentUser?.avatar}
              fallback={avatarFallbackText(currentUser?.avatar, name, currentUser?.id)}
            />
          </span>
          {iconOnly ? null : (
            <>
              <span className={styles.identity}>
                <span className={classNames(styles.name, "truncate")}>{name}</span>
                <span
                  className={classNames(styles.hint, "truncate")}
                  data-connected={openCSG.authenticated && !openCSG.expired}
                >
                  <span className={styles.hintDot} aria-hidden="true" />
                  {openCSG.expired
                    ? t("csghubReauthorize")
                    : openCSG.authenticated
                      ? t("iaUserMenuOpenCSGConnected")
                      : t("iaUserMenuOpenCSGDisconnected")}
                </span>
              </span>
              <span className={styles.caret} aria-hidden="true">
                <ChevronUp />
              </span>
            </>
          )}
        </button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="start" side="top" sideOffset={8} style={{ minWidth: 232 }}>
        <div className={styles.menuHeader}>
          <span className={classNames(styles.menuHeaderName, "truncate")}>{name}</span>
          <span className={classNames(styles.menuHeaderMeta, "truncate")}>
            {[currentUser?.id, role, t("iaUserMenuLocalIdentity")].filter(Boolean).join(" · ")}
          </span>
        </div>
        <DropdownMenuSeparator />

        <DropdownMenuItem disabled>
          <span className={styles.menuSubmenuLabel}>{t("iaUserMenuOpenCSGGroup")}</span>
        </DropdownMenuItem>
        {openCSG.authenticated ? (
          <>
            <DropdownMenuItem disabled>
              <span className={styles.menuIcon} aria-hidden="true">
                <span className={styles.openCSGDot} data-connected={!openCSG.expired} />
              </span>
              <span className={styles.menuSubmenuLabel}>
                {[t(openCSG.expired ? "csghubReauthorize" : "iaUserMenuOpenCSGConnected"), openCSG.baseURL]
                  .filter(Boolean)
                  .join(" · ")}
              </span>
            </DropdownMenuItem>
            {openCSG.expired ? (
              <DropdownMenuItem onSelect={onOpenCSGLogin}>{t("csghubReauthorize")}</DropdownMenuItem>
            ) : null}
            <DropdownMenuItem onSelect={onOpenCSGLogout}>
              <span className={styles.menuIcon} aria-hidden="true">
                <LogOut />
              </span>
              {t("iaUserMenuOpenCSGLogout")}
            </DropdownMenuItem>
          </>
        ) : (
          <DropdownMenuItem onSelect={onOpenCSGLogin}>
            <span className={styles.menuIcon} aria-hidden="true">
              <span className={styles.openCSGDot} data-connected="false" />
            </span>
            {t("iaUserMenuOpenCSGLogin")}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />

        <DropdownMenuItem onSelect={onOpenProfile}>
          <span className={styles.menuIcon} aria-hidden="true">
            <User />
          </span>
          {t("iaUserMenuProfileLocal")}
        </DropdownMenuItem>

        <DropdownMenuItem onSelect={onOpenSettings}>
          <span className={styles.menuIcon} aria-hidden="true">
            <SettingsIcon />
          </span>
          {t("settings")}
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem disabled>
          <span className={styles.menuSubmenuLabel}>{t("themeSwitcher")}</span>
        </DropdownMenuItem>
        {THEME_OPTIONS.map((option) => (
          <DropdownMenuItem
            key={option.key}
            onSelect={(event) => {
              event.preventDefault();
              onThemeChange(option.key);
            }}
          >
            <span className={styles.menuIcon} aria-hidden="true">
              <option.icon />
            </span>
            <span className={styles.menuSubmenuLabel}>{t(option.labelKey)}</span>
            {theme === option.key ? <Check size={14} aria-hidden="true" /> : null}
          </DropdownMenuItem>
        ))}

        <DropdownMenuSeparator />

        <DropdownMenuItem disabled>
          <span className={styles.menuSubmenuLabel}>{t("languageSwitcher")}</span>
        </DropdownMenuItem>
        {(
          [
            { key: "zh" as LocaleCode, labelKey: "languageOptionZh" as const },
            { key: "en" as LocaleCode, labelKey: "languageOptionEn" as const },
          ] satisfies Array<{ key: LocaleCode; labelKey: "languageOptionZh" | "languageOptionEn" }>
        ).map((option) => (
          <DropdownMenuItem
            key={option.key}
            onSelect={(event) => {
              event.preventDefault();
              onLocaleChange(option.key);
            }}
          >
            <span className={styles.menuSubmenuLabel}>{t(option.labelKey)}</span>
            {locale === option.key ? <Check size={14} aria-hidden="true" /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenuRoot>
  );
}
