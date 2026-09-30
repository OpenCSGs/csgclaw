import { imEventsConnected, subscribeIMConnection } from "@/shared/realtime/imEvents";
import { memo, useEffect, useId, useMemo, useRef, useState, useSyncExternalStore } from "react";
import type { ReactNode } from "react";
import { ChevronDown, ChevronRight, Clock3 } from "lucide-react";
import { Button } from "@/components/ui";
import { progressActive, progressDuration, progressGroups, toolGroupLabel, terminalTool } from "@/models/turnProgress";
import type { ProgressGroup, ProgressTool, TurnProgress as Progress } from "@/models/turnProgress";
import { videoProgressTiming } from "@/models/videoGeneration";
import type { IMMessage, TranslateFn } from "@/models/conversations";
import styles from "./TurnProgress.module.css";

type ActionIconProps = {
  "aria-hidden"?: boolean | "true" | "false";
  className?: string;
  size?: number;
};

function ActionIcon({ children, size = 16, ...props }: ActionIconProps & { children: ReactNode }) {
  return (
    <svg
      {...props}
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {children}
    </svg>
  );
}

function SearchActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M14 14L11.1 11.1M12.6667 7.33333C12.6667 10.2789 10.2789 12.6667 7.33333 12.6667C4.38781 12.6667 2 10.2789 2 7.33333C2 4.38781 4.38781 2 7.33333 2C10.2789 2 12.6667 4.38781 12.6667 7.33333Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

function ReadActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M1.61342 8.47545C1.52262 8.33169 1.47723 8.25981 1.45182 8.14894C1.43273 8.06567 1.43273 7.93433 1.45182 7.85106C1.47723 7.74019 1.52262 7.66831 1.61341 7.52455C2.36369 6.33656 4.59693 3.33333 8.00027 3.33333C11.4036 3.33333 13.6369 6.33656 14.3871 7.52455C14.4779 7.66831 14.5233 7.74019 14.5487 7.85106C14.5678 7.93433 14.5678 8.06567 14.5487 8.14894C14.5233 8.25981 14.4779 8.33169 14.3871 8.47545C13.6369 9.66344 11.4036 12.6667 8.00027 12.6667C4.59693 12.6667 2.36369 9.66344 1.61342 8.47545Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M8.00027 10C9.10484 10 10.0003 9.10457 10.0003 8C10.0003 6.89543 9.10484 6 8.00027 6C6.8957 6 6.00027 6.89543 6.00027 8C6.00027 9.10457 6.8957 10 8.00027 10Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

function ListActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M14 8L6 8M14 4L6 4M14 12L6 12M3.33333 8C3.33333 8.36819 3.03486 8.66667 2.66667 8.66667C2.29848 8.66667 2 8.36819 2 8C2 7.63181 2.29848 7.33333 2.66667 7.33333C3.03486 7.33333 3.33333 7.63181 3.33333 8ZM3.33333 4C3.33333 4.36819 3.03486 4.66667 2.66667 4.66667C2.29848 4.66667 2 4.36819 2 4C2 3.63181 2.29848 3.33333 2.66667 3.33333C3.03486 3.33333 3.33333 3.63181 3.33333 4ZM3.33333 12C3.33333 12.3682 3.03486 12.6667 2.66667 12.6667C2.29848 12.6667 2 12.3682 2 12C2 11.6318 2.29848 11.3333 2.66667 11.3333C3.03486 11.3333 3.33333 11.6318 3.33333 12Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

function EditActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M8 13.3333H14M2.00002 13.3333H3.11638C3.4425 13.3333 3.60556 13.3333 3.75901 13.2965C3.89506 13.2638 4.02512 13.21 4.14441 13.1368C4.27897 13.0544 4.39427 12.9391 4.62487 12.7085L13 4.33333C13.5523 3.78104 13.5523 2.88561 13 2.33333C12.4478 1.78104 11.5523 1.78104 11 2.33333L2.62486 10.7085C2.39425 10.9391 2.27895 11.0544 2.1965 11.1889C2.12339 11.3082 2.06952 11.4383 2.03686 11.5744C2.00002 11.7278 2.00002 11.8909 2.00002 12.217V13.3333Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

function ExecuteActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M2.66667 11.3333L6.66667 7.33333L2.66667 3.33333M8 12.6667H13.3333"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

function WebActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <g clipPath="url(#turn-progress-web-icon-clip)">
        <path
          d="M1.33333 8H14.6667M1.33333 8C1.33333 11.6819 4.3181 14.6667 8 14.6667M1.33333 8C1.33333 4.3181 4.3181 1.33333 8 1.33333M14.6667 8C14.6667 11.6819 11.6819 14.6667 8 14.6667M14.6667 8C14.6667 4.3181 11.6819 1.33333 8 1.33333M8 1.33333C9.66752 3.1589 10.6152 5.52802 10.6667 8C10.6152 10.472 9.66752 12.8411 8 14.6667M8 1.33333C6.33248 3.1589 5.38483 5.52802 5.33333 8C5.38483 10.472 6.33248 12.8411 8 14.6667"
          stroke="currentColor"
          strokeWidth="1.33333"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
      <defs>
        <clipPath id="turn-progress-web-icon-clip">
          <rect width="16" height="16" fill="white" />
        </clipPath>
      </defs>
    </ActionIcon>
  );
}

function LoadActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M6 12.3333H10M4.4 1.33333H11.6C11.9734 1.33333 12.1601 1.33333 12.3027 1.406C12.4281 1.46991 12.5301 1.5719 12.594 1.69734C12.6667 1.83995 12.6667 2.02663 12.6667 2.4V3.78301C12.6667 4.10913 12.6667 4.27219 12.6298 4.42564C12.5972 4.56169 12.5433 4.69175 12.4702 4.81105C12.3877 4.9456 12.2724 5.0609 12.0418 5.29151L10.0876 7.24575C9.82357 7.50976 9.69156 7.64177 9.6421 7.79399C9.5986 7.92788 9.5986 8.07212 9.6421 8.20601C9.69156 8.35823 9.82357 8.49024 10.0876 8.75425L12.0418 10.7085C12.2724 10.9391 12.3877 11.0544 12.4702 11.189C12.5433 11.3083 12.5972 11.4383 12.6298 11.5744C12.6667 11.7278 12.6667 11.8909 12.6667 12.217V13.6C12.6667 13.9734 12.6667 14.1601 12.594 14.3027C12.5301 14.4281 12.4281 14.5301 12.3027 14.594C12.1601 14.6667 11.9734 14.6667 11.6 14.6667H4.4C4.02663 14.6667 3.83995 14.6667 3.69734 14.594C3.5719 14.5301 3.46991 14.4281 3.406 14.3027C3.33333 14.1601 3.33333 13.9734 3.33333 13.6V12.217C3.33333 11.8909 3.33333 11.7278 3.37017 11.5744C3.40284 11.4383 3.45671 11.3083 3.52981 11.189C3.61227 11.0544 3.72757 10.9391 3.95817 10.7085L5.91242 8.75425C6.17643 8.49024 6.30844 8.35823 6.3579 8.20601C6.4014 8.07212 6.4014 7.92788 6.3579 7.79399C6.30844 7.64177 6.17643 7.50976 5.91242 7.24575L3.95817 5.29151C3.72757 5.0609 3.61227 4.9456 3.52981 4.81105C3.45671 4.69175 3.40284 4.56169 3.37017 4.42564C3.33333 4.27219 3.33333 4.10913 3.33333 3.78301V2.4C3.33333 2.02663 3.33333 1.83995 3.406 1.69734C3.46991 1.5719 3.5719 1.46991 3.69734 1.406C3.83995 1.33333 4.02663 1.33333 4.4 1.33333Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

function ToolActionIcon(props: ActionIconProps) {
  return (
    <ActionIcon {...props}>
      <path
        d="M10.4209 5.08758C10.1569 4.82357 10.0249 4.69156 9.97544 4.53934C9.93193 4.40545 9.93193 4.26122 9.97544 4.12732C10.0249 3.9751 10.1569 3.8431 10.4209 3.57909L12.3132 1.68684C11.811 1.45975 11.2536 1.33333 10.6667 1.33333C8.45753 1.33333 6.66667 3.12419 6.66667 5.33333C6.66667 5.66069 6.70599 5.97887 6.78018 6.28339C6.85961 6.60949 6.89933 6.77255 6.89228 6.87555C6.8849 6.98339 6.86882 7.04077 6.81909 7.13674C6.77159 7.22841 6.68057 7.31943 6.49855 7.50145L2.33333 11.6667C1.78105 12.219 1.78105 13.1144 2.33333 13.6667C2.88562 14.219 3.78105 14.219 4.33333 13.6667L8.49855 9.50145C8.68057 9.31943 8.77159 9.22842 8.86326 9.18091C8.95923 9.13118 9.01661 9.1151 9.12445 9.10772C9.22746 9.10067 9.39051 9.14039 9.71661 9.21983C10.0211 9.29401 10.3393 9.33333 10.6667 9.33333C12.8758 9.33333 14.6667 7.54247 14.6667 5.33333C14.6667 4.74639 14.5403 4.18898 14.3132 3.68684L12.4209 5.57909C12.1569 5.8431 12.0249 5.9751 11.8727 6.02456C11.7388 6.06807 11.5946 6.06807 11.4607 6.02456C11.3084 5.9751 11.1764 5.8431 10.9124 5.57909L10.4209 5.08758Z"
        stroke="currentColor"
        strokeWidth="1.33333"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </ActionIcon>
  );
}

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
  const toggle = () => {
    setDisclosure({ id: progress.id, active, open: !open });
  };
  return (
    <section className={styles.shell} ref={shell} data-turn-id={progress.id} data-preserve-scroll={open || undefined}>
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
          {group.kind === "tool" ? (
            <GroupToolIcon items={group.items} />
          ) : (
            <Clock3 size={15} aria-hidden="true" />
          )}
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

function GroupToolIcon({ items }: { items: ProgressGroup["items"] }) {
  const action = items.find((item) => item.tool?.actions?.[0])?.tool?.actions[0] || "tool";
  const Icon = toolIcon(action);
  return <Icon size={15} aria-hidden="true" />;
}

const ToolRow = memo(function ToolRow({ tool, number, t }: { tool: ProgressTool; number: number; t: TranslateFn }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const failed = ["failed", "error", "declined"].includes(tool.status);
  const label = t(`progressToolStatus_${tool.status}`);
  const status = label.startsWith("progressToolStatus_") ? t("progressToolStatus_running") : label;
  const preview = (tool.command || tool.input || tool.name).replace(/\s+/g, " ").trim().slice(0, 240);
  const Icon = toolIcon(tool.actions[0] || (tool.command ? "execute" : "tool"));
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
                <ListActionIcon size={13} aria-hidden="true" />
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
      return ReadActionIcon;
    case "search":
      return SearchActionIcon;
    case "list":
      return ListActionIcon;
    case "edit":
      return EditActionIcon;
    case "execute":
      return ExecuteActionIcon;
    case "web":
      return WebActionIcon;
    case "load":
      return LoadActionIcon;
    default:
      return ToolActionIcon;
  }
}
