import { useLocation, useNavigate, useNavigationType } from "react-router-dom";
import { useEffect, useState, type PropsWithChildren } from "react";
import { ArrowLeft, ArrowRight, PanelLeft } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import { getDesktopBridge, type DesktopPlatform } from "@/shared/platform/desktopBridge";
import type { TranslateFn } from "@/models/conversations";
import { createTranslator } from "@/shared/i18n";
import styles from "./DesktopChrome.module.css";

export function DesktopChrome({
  children,
  title,
  collapsed,
  onToggleSidebar,
  t = createTranslator("en"),
}: PropsWithChildren<{
  title?: string;
  collapsed: boolean;
  onToggleSidebar?: () => void;
  t?: TranslateFn;
}>) {
  const bridge = getDesktopBridge();
  const [platform, setPlatform] = useState<DesktopPlatform>(() =>
    navigator.platform.startsWith("Mac") ? "darwin" : navigator.platform.startsWith("Win") ? "win32" : "linux",
  );
  const [focused, setFocused] = useState(true);
  useEffect(() => {
    if (!bridge) return;
    let alive = true;
    void bridge
      .getRuntimeInfo()
      .then((info) => {
        if (alive) setPlatform(info.platform);
      })
      .catch(() => {});
    void bridge
      .getWindowState()
      .then((state) => {
        if (alive) setFocused(state.focused);
      })
      .catch(() => {});
    const unsubscribe = bridge.onWindowState((state) => setFocused(state.focused));
    return () => {
      alive = false;
      unsubscribe();
    };
  }, [bridge]);
  if (!bridge) return <>{children}</>;
  return (
    <div className={styles.frame} data-desktop-platform={platform}>
      <header className={styles.titlebar} data-focused={focused} aria-label={t("desktopTitlebar")}>
        <div className={styles.leading}>
          <DesktopHistoryControls t={t} />
          <Tooltip content={t(collapsed ? "expandSidebar" : "collapseSidebar")}>
            <Button
              size="sm"
              variant="tertiaryGray"
              iconOnly
              aria-label={t(collapsed ? "expandSidebar" : "collapseSidebar")}
              aria-expanded={!collapsed}
              disabled={!onToggleSidebar}
              onClick={onToggleSidebar}
            >
              <PanelLeft size={16} />
            </Button>
          </Tooltip>
        </div>
        <div className={styles.identity}>
          <strong>CSGClaw</strong>
          {title ? (
            <>
              <span className={styles.divider}>/</span>
              <span className={styles.context} title={title}>
                {title}
              </span>
            </>
          ) : null}
        </div>
        <div className={styles.trailing} aria-hidden="true" />
      </header>
      <div className={styles.content}>{children}</div>
    </div>
  );
}

function DesktopHistoryControls({ t }: { t: TranslateFn }) {
  const location = useLocation();
  const navigationType = useNavigationType();
  const navigate = useNavigate();
  const [history, setHistory] = useState(() => {
    const index = Number(window.history.state?.idx) || 0;
    return { index, maximum: index };
  });
  useEffect(() => {
    const index = Number(window.history.state?.idx) || 0;
    setHistory((current) => ({ index, maximum: navigationType === "PUSH" ? index : Math.max(current.maximum, index) }));
  }, [location.key, navigationType]);
  return (
    <>
      <Tooltip content={t("desktopBack")}>
        <Button
          size="sm"
          variant="tertiaryGray"
          iconOnly
          aria-label={t("desktopBack")}
          disabled={history.index === 0}
          onClick={() => navigate(-1)}
        >
          <ArrowLeft size={16} />
        </Button>
      </Tooltip>
      <Tooltip content={t("desktopForward")}>
        <Button
          size="sm"
          variant="tertiaryGray"
          iconOnly
          aria-label={t("desktopForward")}
          disabled={history.index >= history.maximum}
          onClick={() => navigate(1)}
        >
          <ArrowRight size={16} />
        </Button>
      </Tooltip>
    </>
  );
}
