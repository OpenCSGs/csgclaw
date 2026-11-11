import { useState, type CSSProperties } from "react";
import { Button, DismissibleAlert, GlobalNoticeProvider, SegmentedControl, useGlobalNotice } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import alertInfo from "./alert-assets/alert-info.svg";
import alertWarning from "./alert-assets/alert-warning.svg";
import warning from "./alert-assets/warning.svg";
import desktopIcon from "./alert-assets/notification-desktop.svg";
import mobileIcon from "./alert-assets/notification-mobile.svg";
import closeIcon from "./alert-assets/close.svg";
import bannerClose from "./alert-assets/close-banner.svg";
import desktopBackground from "./alert-assets/desktop.png";
import mobileBackground from "./alert-assets/mobile.png";
import styles from "./AlertNotificationDemos.module.css";

const figma = (id: string) => `https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/?node-id=${id}`;

function FeedbackIcon({ src }: { src: string }) {
  return (
    <span
      className={styles.glyph}
      style={{ "--feedback-glyph-mask": `url("${src}")` } as CSSProperties}
      aria-hidden="true"
    >
      <img src={src} alt="" />
    </span>
  );
}

function FeaturedIcon({ src, kind }: { src: string; kind: "default" | "warning" | "brand" }) {
  return (
    <span className={styles.featured} data-icon={kind} aria-hidden="true">
      <FeedbackIcon src={src} />
    </span>
  );
}

function NotificationTriggers({ mobile }: { mobile: boolean }) {
  const { text: t } = useLabTranslation();
  const { showNotice } = useGlobalNotice();
  const [feedback, setFeedback] = useState("");
  function show(isWarning: boolean) {
    setFeedback("");
    const label = isWarning ? "撤销操作" : "更新日志";
    showNotice({
      tone: isWarning ? "warning" : "info",
      title: t(isWarning ? "项目已取消发布" : "我们刚刚发布了一项更新！"),
      message: t(
        isWarning
          ? "移除所有用户后，项目已取消发布。添加用户即可重新发布。"
          : "试试全新的仪表盘。页面和导出的加载速度更快了。",
      ),
      icon: (
        <FeaturedIcon
          src={isWarning ? warning : mobile ? mobileIcon : desktopIcon}
          kind={isWarning ? "warning" : "brand"}
        />
      ),
      closeLabel: t("关闭通知"),
      dismissLabel: isWarning ? undefined : t("忽略"),
      action: { label: t(label), onSelect: () => setFeedback(label) },
    });
  }
  return (
    <>
      <div className={styles.controls}>
        <Button onClick={() => show(false)}>{t("触发品牌通知")}</Button>
        <Button onClick={() => show(true)}>{t("触发警告通知")}</Button>
      </div>
      <p role="status" className={styles.feedback}>
        {feedback && t(feedback)}
      </p>
    </>
  );
}

function NotificationPreview({ mobile }: { mobile: boolean }) {
  const { text: t } = useLabTranslation();
  return (
    <section className={styles.section}>
      <h3>{t("全局通知")}</h3>
      <p>{t("触发通知以体验自动消失、关闭和后续操作。")}</p>
      <div className={styles.controls}>
        <a href={figma(mobile ? "1135-914" : "1135-591")} target="_blank" rel="noreferrer">
          {t("品牌通知设计稿")}
        </a>
        <a href={figma(mobile ? "1135-2294" : "1135-2309")} target="_blank" rel="noreferrer">
          {t("警告通知设计稿")}
        </a>
      </div>
      <GlobalNoticeProvider
        key={String(mobile)}
        noticeClassName={styles.toast}
        viewportClassName={styles.toastViewport}
        closeIcon={<FeedbackIcon src={closeIcon} />}
      >
        <NotificationTriggers mobile={mobile} />
        <div className={styles.notificationStage} data-mobile={mobile}>
          <img src={mobile ? mobileBackground : desktopBackground} alt="" />
        </div>
      </GlobalNoticeProvider>
    </section>
  );
}

export function AlertNotificationDemos() {
  const { text: t } = useLabTranslation();
  const [breakpoint, setBreakpoint] = useState("desktop");
  const [variant, setVariant] = useState("warning");
  const [version, setVersion] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [help, setHelp] = useState(false);
  const mobile = breakpoint === "mobile";
  const banner = variant === "banner";
  const defaultAlert = variant === "default";
  const source = banner
    ? mobile
      ? "4893-412529"
      : "4893-412516"
    : defaultAlert
      ? "4050-412599"
      : mobile
        ? "1130-82749"
        : "1130-82860";
  function reset() {
    setDismissed(false);
    setHelp(false);
    setVersion((previous) => previous + 1);
  }
  return (
    <div className={styles.demos}>
      <SegmentedControl
        value={breakpoint}
        onChange={(value) => {
          setBreakpoint(value);
          if (value === "mobile" && variant === "default") setVariant("warning");
          reset();
        }}
        label={t("设计断点")}
        options={[
          { value: "desktop", label: t("桌面端") },
          { value: "mobile", label: t("移动端") },
        ]}
      />
      <section className={styles.section}>
        <h3>{t("行内提醒")}</h3>
        <div className={styles.controls}>
          <SegmentedControl
            value={variant}
            onChange={(value) => {
              setVariant(value);
              reset();
            }}
            label={t("提醒样式")}
            options={[
              { value: "warning", label: t("浮动警告") },
              ...(!mobile ? [{ value: "default", label: t("浮动默认") }] : []),
              { value: "banner", label: t("通栏警告") },
            ]}
          />
          <Button onClick={reset}>{t("重新显示提醒")}</Button>
          <a href={figma(source)} target="_blank" rel="noreferrer">
            {t("提醒设计稿")}
          </a>
        </div>
        <div className={styles.alertStage} data-mobile={mobile} data-banner={banner}>
          {!dismissed && (
            <DismissibleAlert
              messageKey={String(version)}
              className={styles.alert}
              closeLabel={t("关闭提醒")}
              onDismiss={() => setHelp(false)}
              closeIcon={<FeedbackIcon src={banner && !mobile ? bannerClose : closeIcon} />}
            >
              <div className={styles.alertBody}>
                {(!banner || !mobile) && (
                  <FeaturedIcon
                    src={defaultAlert ? alertInfo : banner ? warning : alertWarning}
                    kind={defaultAlert ? "default" : "warning"}
                  />
                )}
                <div className={styles.alertCopy}>
                  <p>{t(defaultAlert ? "我们刚刚发布了一项新功能" : "请注意，这可能会带来问题")}</p>
                  <p>
                    {t(
                      banner ? "请查看相关说明，确认后再继续。" : "请查看相关说明。如有疑问，可以在继续之前确认详情。",
                    )}
                  </p>
                </div>
                <div className={styles.actions}>
                  <Button
                    variant="linkGray"
                    onClick={() => {
                      setDismissed(true);
                      setHelp(false);
                    }}
                  >
                    {t("忽略")}
                  </Button>
                  <Button variant="linkColor" aria-expanded={help} onClick={() => setHelp(!help)}>
                    {t(defaultAlert ? "查看变更" : "了解更多")}
                  </Button>
                </div>
              </div>
            </DismissibleAlert>
          )}
        </div>
        {help && !dismissed && <p role="status">{t("此提醒仅用于展示，不会修改真实设置。")}</p>}
      </section>
      <NotificationPreview mobile={mobile} />
    </div>
  );
}
