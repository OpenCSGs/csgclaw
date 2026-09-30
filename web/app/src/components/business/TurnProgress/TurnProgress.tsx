import { imEventsConnected, subscribeIMConnection } from "@/shared/realtime/imEvents";
import { memo, useEffect, useId, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { ReactNode } from "react";
import {
  ChevronDown,
  ChevronRight,
  Clock3,
  Wrench,
  Terminal,
  FileText,
  Search,
  Pencil,
  Globe,
  Folder,
} from "lucide-react";
import { Button } from "@/components/ui";
import { progressActive, progressDuration, progressGroups, toolGroupLabel, terminalTool } from "@/models/turnProgress";
import type { ProgressGroup, ProgressTool, TurnProgress as Progress } from "@/models/turnProgress";
import { videoProgressTiming } from "@/models/videoGeneration";
import type { IMMessage, TranslateFn } from "@/models/conversations";
import styles from "./TurnProgress.module.css";

type Props = {
  videoMessages?: IMMessage[];
  controls?: ReactNode;
  headerControls?: ReactNode;
  progress: Progress;
  answer?: ReactNode;
  renderText: (text: string) => ReactNode;
  t: TranslateFn;
};

export const TurnProgress = memo(function TurnProgress({
  progress,
  answer,
  renderText,
  t,
  controls,
  headerControls,
  videoMessages = [],
}: Props) {
  const connected = useSyncExternalStore(subscribeIMConnection, imEventsConnected);
  const active = progressActive(progress);
  const videoTiming = videoProgressTiming(progress, videoMessages);
  const clockProgress = videoTiming?.progress ?? progress;
  const [disclosure, setDisclosure] = useState({ id: progress.id, active, open: active });
  if (disclosure.id !== progress.id || disclosure.active !== active) {
    setDisclosure({ id: progress.id, active, open: active });
  }
  const open = disclosure.id === progress.id && disclosure.active === active ? disclosure.open : active;
  const shell = useRef<HTMLElement>(null);
  const id = useId();
  const groups = useMemo(() => progressGroups(progress.items), [progress.items]);
  const hasProcess = groups.length > 0 || active || videoTiming !== null || progress.status !== "succeeded";
  const [online, setOnline] = useState(() => typeof navigator === "undefined" || navigator.onLine);
  const statusLabel =
    !active && videoTiming
      ? videoTiming.pending
        ? "videoGenerating"
        : videoTiming.failed
          ? "videoGenerationFailed"
          : progress.status === "succeeded"
            ? "progressTotalTime"
            : `progressStatus_${progress.status}`
      : `progressStatus_${active && (!online || !connected) ? "reconnecting" : progress.status}`;
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
    };
  }, []);
  // Preserve the outer transcript anchor when a user toggles the process.
  const toggle = () => {
    const el = shell.current;
    const top = el?.getBoundingClientRect().top;
    setDisclosure({ id: progress.id, active, open: !open });
    requestAnimationFrame(() => {
      const parent = el?.closest<HTMLElement>(".messages, .thread-panel-body");
      if (parent && top !== undefined && el) parent.scrollTop += el.getBoundingClientRect().top - top;
    });
  };
  return (
    <section className={styles.shell} ref={shell} data-turn-id={progress.id}>
      {hasProcess ? (
        <>
          <div className={styles.header}>
            <Button
              variant="linkGray"
              size="sm"
              className={styles.heading}
              aria-expanded={open}
              aria-controls={id}
              onClick={toggle}
            >
              <Clock3 size={15} aria-hidden="true" />
              <span>{t(statusLabel)}</span>
              {clockProgress.status !== "succeeded" ? <span aria-hidden="true">·</span> : null}
              <Elapsed progress={clockProgress} />
              {open ? <ChevronDown size={15} aria-hidden="true" /> : <ChevronRight size={15} aria-hidden="true" />}
            </Button>
            {active && headerControls ? <div className={styles.headerControls}>{headerControls}</div> : null}
          </div>
          {open ? (
            <div id={id} className={styles.body} role="region" aria-label={t("progressDetails")}>
              {groups.map((group) => (
                <Group key={group.id} group={group} renderText={renderText} t={t} />
              ))}
              {!groups.length ? <p className={styles.waiting}>{t("progressStarting")}</p> : null}
            </div>
          ) : null}
        </>
      ) : null}
      {progress.error ? (
        <p className={styles.error} role="status">
          {progress.error}
        </p>
      ) : null}
      {answer ? <div className={styles.answer}>{answer}</div> : null}
      {active ? controls : null}
    </section>
  );
});
function Elapsed({ progress }: { progress: Progress }) {
  const [now, setNow] = useState(() => Date.now());
  const active = progressActive(progress);
  useEffect(() => {
    if (!active) return;
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [active]);
  return <span className={styles.elapsed}>{progressDuration(progress, now)}</span>;
}
const Group = memo(
  function Group({ group, renderText, t }: { group: ProgressGroup; renderText: Props["renderText"]; t: TranslateFn }) {
    if (group.kind === "commentary")
      return (
        <div className={styles.commentary} data-diagnostic-response-text>
          {renderText(group.items[0].text || "")}
        </div>
      );
    return (
      <div className={styles.group}>
        <div className={styles.summary}>
          {group.kind === "tool" ? <Wrench size={15} aria-hidden="true" /> : <Clock3 size={15} aria-hidden="true" />}
          <span>{group.kind === "tool" ? toolGroupLabel(group.items, t) : t("progressReasoning")}</span>
        </div>
        <div className={styles.details}>
          {group.kind === "reasoning" ? (
            renderText(group.items[0].text || "")
          ) : (
            <ol className={styles.toolList} start={group.firstToolNumber} aria-label={t("progressToolList")}>
              {group.items.map((item, index) =>
                item.tool ? (
                  <ToolRow key={item.id} tool={item.tool} number={group.firstToolNumber + index} t={t} />
                ) : null,
              )}
            </ol>
          )}
        </div>
      </div>
    );
  },
  (previous, next) =>
    previous.t === next.t &&
    previous.renderText === next.renderText &&
    previous.group.kind === next.group.kind &&
    previous.group.firstToolNumber === next.group.firstToolNumber &&
    previous.group.items.length === next.group.items.length &&
    previous.group.items.every((item, index) => item === next.group.items[index]),
);

const ToolRow = memo(function ToolRow({ tool, number, t }: { tool: ProgressTool; number: number; t: TranslateFn }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const failed = ["failed", "error", "declined"].includes(tool.status);
  const label = t(`progressToolStatus_${tool.status}`);
  const status = label.startsWith("progressToolStatus_") ? t("progressToolStatus_running") : label;
  const preview = (tool.command || tool.input || tool.name).replace(/\s+/g, " ").trim().slice(0, 240);
  const Icon = tool.command ? Terminal : toolIcon(tool.actions[0]);
  return (
    <li className={styles.tool} data-failed={failed || undefined}>
      <Button
        variant="linkGray"
        size="sm"
        className={styles.toolToggle}
        aria-expanded={open}
        aria-controls={id}
        aria-label={t("progressToolDetails", { number, command: preview, status })}
        onClick={() => {
          setOpen(!open);
        }}
      >
        <span className={styles.ordinal} aria-hidden="true">
          {number}
        </span>
        <Icon size={15} aria-hidden="true" />
        <span className={styles.commandPreview} title={preview}>
          {preview}
        </span>
        <span className={styles.toolStatus}>{status}</span>
        {open ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
      </Button>
      {open ? (
        <div id={id} className={styles.toolDetails}>
          {tool.command || tool.input ? (
            <div className={styles.detailSection}>
              <span className={styles.detailLabel}>{t(tool.command ? "progressCommand" : "progressInput")}</span>
              <pre>{tool.command || tool.input}</pre>
            </div>
          ) : null}
          <div className={styles.detailSection}>
            <span className={styles.detailLabel}>{t("progressOutput")}</span>
            {tool.output ? (
              <pre>{tool.output}</pre>
            ) : (
              <p className={styles.emptyOutput}>
                {t(terminalTool(tool.status) ? "progressNoOutput" : "progressWaitingOutput")}
              </p>
            )}
            {tool.truncated ? <small>{t("progressTruncated")}</small> : null}
          </div>
          <div className={styles.toolMeta}>
            {tool.exit_code != null ? <span>{t("progressExitCode", { code: tool.exit_code })}</span> : null}
            {tool.duration_ms != null ? (
              <span>{t("progressToolDuration", { seconds: (tool.duration_ms / 1000).toFixed(1) })}</span>
            ) : null}
            {tool.cwd ? (
              <span className={styles.directory} title={tool.cwd}>
                <Folder size={13} aria-hidden="true" />
                <span>{t("progressDirectory", { path: tool.cwd })}</span>
              </span>
            ) : null}
          </div>
        </div>
      ) : null}
    </li>
  );
});

function toolIcon(action: string) {
  switch (action) {
    case "read":
      return FileText;
    case "search":
      return Search;
    case "list":
      return Folder;
    case "edit":
      return Pencil;
    case "web":
      return Globe;
    default:
      return Wrench;
  }
}
