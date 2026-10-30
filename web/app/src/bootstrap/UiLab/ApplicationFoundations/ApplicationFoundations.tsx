import { applicationSections, type ApplicationKind } from "./registry";
import { FigmaReference } from "./FigmaReference";
import { HeaderDemo, NavigationDemo, BreadcrumbDemo, InlineCtaDemo, DividerDemo } from "./LayoutDemos";
import { ModalDemo, CommandDemo } from "./OverlayDemos";
import { TableDemo, MetricDemo, ChartDemo, ActivityDemo } from "./DataDemos";
import { DatePickerDemo, CalendarDemo } from "./DateDemos";
import { AlertDemo, LoadingDemo, EmptyDemo } from "./FeedbackDemos";
import { MessagingDemo, UploadDemo, CodeDemo } from "./ContentDemos";
import { useLabTranslation } from "../LabLocale";
import inventory from "./figmaInventory.json";
import styles from "./ApplicationFoundations.module.css";
function Demo({ kind }: { kind: ApplicationKind }) {
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
      return <MessagingDemo />;
    case "tables":
      return <TableDemo />;
    case "breadcrumbs":
      return <BreadcrumbDemo />;
    case "alerts":
      return <AlertDemo />;
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
export function ApplicationFoundations({ kind }: { kind: ApplicationKind }) {
  const { text: t } = useLabTranslation();
  const entry = applicationSections.find((section) => section.key === kind)!;
  const source = inventory.find((item) => item.key === kind)!;
  return (
    <section id={`${kind}-foundations`} className={styles.foundation} aria-labelledby={`${kind}-title`}>
      <h2 id={`${kind}-title`}>{t(`${entry.title} · ${entry.zh}`)}</h2>
      <p>{t(entry.description)}</p>
      <a
        href={`https://www.figma.com/design/2s3AVSZ5FfBnCR5nI2AriR/?node-id=${source.pageId.replace(":", "-")}`}
        target="_blank"
        rel="noreferrer"
      >
        {t("查看 Figma 设计来源 ↗")}
      </a>
      <p>{t("本页已收录一个代表性设计节点与交互试用。完整变体矩阵及生产页面迁移尚待验收。")}</p>
      <section
        className={`${styles.stage} ${styles.demoSurface}`}
        aria-label={`${t(entry.title + " · " + entry.zh)} · ${t("交互试用")}`}
      >
        <h3>{t("交互试用")}</h3>
        <p>{t("试用区使用本地示例数据，不连接业务服务。设计原始文案与尺寸见下方参考。")}</p>
        <Demo kind={kind} />
      </section>
      <FigmaReference kind={kind} />
    </section>
  );
}
