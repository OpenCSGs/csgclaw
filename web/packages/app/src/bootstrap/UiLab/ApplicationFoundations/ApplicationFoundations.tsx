import canvasStyles from "../LabPreviewCanvas.module.css";
import { LabComponentHeading } from "../LabComponentHeading";
import { DesignOnly, PreviewOnly } from "../LabView";
import { useState } from "react";
import { Button, LinkButton } from "@/components/ui";
import { applicationSections, type ApplicationKind } from "./registry";
import { FigmaReference } from "./FigmaReference";
import { HeaderDemo, NavigationDemo, BreadcrumbDemo, InlineCtaDemo, DividerDemo } from "./LayoutDemos";
import { ModalDemo, CommandDemo } from "./OverlayDemos";
import { TableDemo, MetricDemo, ChartDemo, ActivityDemo } from "./DataDemos";
import { DatePickerDemo, CalendarDemo } from "./DateDemos";
import { LoadingDemo } from "./FeedbackDemos";
import { EmptyDemo } from "./EmptyDemo";
import { CodeDemo } from "./CodeDemo";
import { UploadDemo } from "./UploadDemo";
import { MessagingDemo } from "./MessagingDemo";
import type { ResolvedThemeMode } from "@/shared/theme/theme";
import { useLabTranslation } from "../LabLocale";
import { AlertNotificationDemos } from "./AlertNotificationDemos";
import inventory from "./figmaInventory.json";
import styles from "./ApplicationFoundations.module.css";
function Demo({ kind, theme }: { kind: ApplicationKind; theme: ResolvedThemeMode }) {
  switch (kind) {
    case "page-headers":
    case "card-headers":
    case "section-headers":
      return <HeaderDemo kind={kind} />;
    case "app-navigation":
      return <NavigationDemo />;
    case "modals":
      return <ModalDemo />;
    case "command-menus":
      return <CommandDemo />;
    case "charts":
      return <ChartDemo />;
    case "metrics":
      return <MetricDemo />;
    case "slideouts":
      return <ModalDemo drawer />;
    case "inline-ctas":
      return <InlineCtaDemo />;
    case "activity-feeds":
      return <ActivityDemo />;
    case "messaging":
      return <MessagingDemo theme={theme} />;
    case "tables":
      return <TableDemo />;
    case "breadcrumbs":
      return <BreadcrumbDemo />;
    case "date-pickers":
      return <DatePickerDemo />;
    case "calendars":
      return <CalendarDemo />;
    case "file-upload":
      return <UploadDemo />;
    case "dividers":
      return <DividerDemo />;
    case "loading":
      return <LoadingDemo />;
    case "empty-states":
      return <EmptyDemo />;
    case "code-snippets":
      return <CodeDemo />;
  }
}
export function ApplicationFoundations({
  kind,
  theme = "light",
}: {
  kind: ApplicationKind;
  theme?: ResolvedThemeMode;
}) {
  const { text: t } = useLabTranslation();
  const [revision, setRevision] = useState(0);
  const entry = applicationSections.find((section) => section.key === kind)!;
  const source = inventory.find((item) => item.key === kind)!;
  const preview = (
    <section
      className={`${canvasStyles.previewCanvas} ${styles.liveStage}${kind === "messaging" ? "" : ` ${styles.demoSurface}`}${kind !== "breadcrumbs" ? ` ${styles.withReset}` : ""}`}
      data-kind={kind}
      aria-label={`${t(entry.title + " · " + entry.zh)} · ${t("交互试用")}`}
    >
      {kind !== "breadcrumbs" && (
        <div className={styles.canvasReset}>
          <Button size="sm" onClick={() => setRevision((value) => value + 1)}>
            {t("重置示例")}
          </Button>
        </div>
      )}
      <Demo key={revision} kind={kind} theme={theme} />
    </section>
  );
  return (
    <section id={`${kind}-foundations`} className={styles.foundation} aria-labelledby={`${kind}-title`}>
      <LabComponentHeading
        sectionId={`${kind}-foundations`}
        titleId={`${kind}-title`}
        designAction={
          <LinkButton
            className={styles.sourceButton}
            variant="primary"
            href={`https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/?node-id=${source.pageId.replace(":", "-")}`}
            target="_blank"
            rel="noreferrer"
          >
            {t("查看 Figma 设计来源 ↗")}
          </LinkButton>
        }
      >
        {t(`${entry.title} · ${entry.zh}`)}
      </LabComponentHeading>
      <p>{t(entry.description)}</p>
      <PreviewOnly>{kind === "alerts" ? <AlertNotificationDemos /> : preview}</PreviewOnly>
      <DesignOnly>
        <FigmaReference kind={kind} />
      </DesignOnly>
    </section>
  );
}
