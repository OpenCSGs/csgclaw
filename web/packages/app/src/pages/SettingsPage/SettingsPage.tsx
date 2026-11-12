import { Fragment, useEffect, useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { DialogCloseButton, DialogContent, DialogRoot, DialogTitle } from "@/components/ui";
import { ChevronLeft } from "lucide-react";
import { useWorkspaceControllerContext, useWorkspaceDialogs } from "@/hooks/workspace";
import { WorkspaceDialogNames } from "@/models/workspaceDialogs";
import { classNames } from "@/shared/lib/classNames";
import {
  DEFAULT_SETTINGS_SECTION,
  SETTINGS_GROUPS,
  settingsSectionFromSearch,
  type SettingsSectionID,
} from "./settingsCatalog";
import { AccountSection } from "./sections/AccountSection";
import { AppearanceSection } from "./sections/AppearanceSection";
import { ArchivedSection } from "./sections/ArchivedSection";
import { ConnectorsSection } from "./sections/ConnectorsSection";
import { KnowledgeBasesSection } from "./sections/KnowledgeBasesSection";
import { SkillsSection } from "./sections/SkillsSection";
import { FeedbackSection } from "./sections/FeedbackSection";
import { ModelServiceSection } from "./sections/ModelServiceSection";
import { NotificationsSection } from "./sections/NotificationsSection";
import { ExternalMessagesSection } from "./sections/ExternalMessagesSection";
import { RuntimeSection } from "./sections/RuntimeSection";
import { ServerSection } from "./sections/ServerSection";
import { VersionSection } from "./sections/VersionSection";
import styles from "./SettingsPage.module.css";

/** Settings uses query parameters so opening and closing preserve the workspace and its draft. */
export function SettingsPage() {
  const dialogRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);
  const controller = useWorkspaceControllerContext();
  const location = useLocation();
  const navigate = useNavigate();
  const { current, close } = useWorkspaceDialogs();

  const open = current === WorkspaceDialogNames.settings;
  const loadConfig = controller.configSettings?.load;
  const section = settingsSectionFromSearch(location.search);
  const setResourceType = controller.sidebarProps?.hub?.setSelectedHubResourceType;
  useEffect(() => {
    if (!open) return;
    if (section === "skills") setResourceType?.("skill");
    if (section === "connectors") setResourceType?.("mcp");
    if (section === "knowledge") setResourceType?.("knowledge");
  }, [open, section, setResourceType]);
  useEffect(() => {
    if (open) void loadConfig?.();
  }, [open, loadConfig]);

  if (!open || !controller.ready || !controller.sidebarProps) {
    return null;
  }
  const t = controller.sidebarProps.t;
  // CSS uses this URL state to switch between catalog and details on narrow screens.
  const navigating = location.search.includes("section=");
  const activeSection = settingsSectionFromSearch(location.search);

  function closeSettings() {
    close();
  }

  // Replace history so Back does not reopen settings after the dialog closes.
  function goToSection(id: SettingsSectionID | null) {
    const next = new URLSearchParams(location.search);
    if (!id || id === DEFAULT_SETTINGS_SECTION) {
      next.delete("section");
    } else {
      next.set("section", id);
    }
    void navigate({ search: next.toString() }, { replace: true });
  }

  return (
    <DialogRoot
      open
      onOpenChange={(next) => {
        if (!next) closeSettings();
      }}
    >
      <DialogContent
        ref={dialogRef}
        className={styles.dialog}
        aria-describedby={undefined}
        onOpenAutoFocus={() => {
          triggerRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (triggerRef.current?.isConnected) triggerRef.current.focus({ preventScroll: true });
        }}
        onEscapeKeyDown={(event) => {
          // A newly opened nested portal can receive focus before Radix updates its layer order.
          if (
            event.target instanceof Element &&
            event.target.closest('[role="dialog"]') &&
            !dialogRef.current?.contains(event.target)
          ) {
            event.preventDefault();
          }
        }}
      >
        <section className={classNames("settings-page", styles.page)} data-navigating={navigating ? "true" : undefined}>
          <header className={styles.header}>
            <DialogTitle>{t("settings")}</DialogTitle>
            <DialogCloseButton label={t("close")} />
          </header>

          <div className={styles.split}>
            <nav className={styles.catalog} aria-label={t("settings")}>
              {SETTINGS_GROUPS.map((group) => (
                <Fragment key={group.id}>
                  <p className={styles.groupLabel}>{t(group.labelKey)}</p>
                  {group.cards.map((card) => {
                    const Icon = card.icon;
                    const active = card.id === activeSection;
                    return (
                      <button
                        key={card.id}
                        type="button"
                        className={classNames(styles.card, active && styles.cardActive)}
                        aria-current={active ? "true" : undefined}
                        onClick={() => goToSection(card.id)}
                      >
                        <span className={styles.cardIcon} aria-hidden="true">
                          <Icon size={17} strokeWidth={1.9} />
                        </span>
                        <span className={styles.cardText}>
                          <span className={styles.cardTitle}>{t(card.titleKey)}</span>
                          <span className={styles.cardHint}>{t(card.hintKey)}</span>
                        </span>
                      </button>
                    );
                  })}
                </Fragment>
              ))}
            </nav>

            <div className={styles.detail}>
              <button type="button" className={styles.backToCatalog} onClick={() => goToSection(null)}>
                <ChevronLeft size={15} strokeWidth={2} aria-hidden="true" />
                {t("iaSettingsBackToCatalog")}
              </button>
              <SettingsSectionBody section={activeSection} />
            </div>
          </div>
        </section>
      </DialogContent>
    </DialogRoot>
  );
}

function SettingsSectionBody({ section }: { section: SettingsSectionID }) {
  switch (section) {
    case "account":
      return <AccountSection />;
    case "models":
      return <ModelServiceSection />;
    case "connectors":
      return <ConnectorsSection />;
    case "skills":
      return <SkillsSection />;
    case "knowledge":
      return <KnowledgeBasesSection />;
    case "external":
      return <ExternalMessagesSection />;
    case "runtime":
      return <RuntimeSection />;
    case "server":
      return <ServerSection />;
    case "appearance":
      return <AppearanceSection />;
    case "notifications":
      return <NotificationsSection />;
    case "archived":
      return <ArchivedSection />;
    case "version":
      return <VersionSection />;
    case "feedback":
      return <FeedbackSection />;
    default:
      return null;
  }
}
