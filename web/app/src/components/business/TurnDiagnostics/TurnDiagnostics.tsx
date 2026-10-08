import { DiagnosticTimeline } from "./DiagnosticTimeline";
import { DiagnosticBreakdown, DiagnosticCalls, DiagnosticSpanFields } from "./DiagnosticAnalysis";
import { observeDiagnosticRender } from "@/shared/diagnostics/renderTiming";
import { useEffect, useRef, useState } from "react";
import { Activity, AlertCircle, ChevronDown, Clock, Copy, Download, RefreshCw } from "lucide-react";
import {
  Button,
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  TooltipProvider,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  DialogBody,
  Select,
  Tooltip,
} from "@/components/ui";
import { fetchDiagnostic, fetchDiagnostics, type TurnDiagnostic } from "@/api/diagnostics";
import { fetchAgentLogsRequest } from "@/api/agents";
import {
  diagnosticDuration,
  diagnosticDefaultHiddenCategories,
  diagnosticTimelineRows,
  diagnosticHiddenStats,
  type DiagnosticHiddenCategory,
  diagnosticSpanLabel,
  diagnosticTimestamp,
  diagnosticOccupied,
  diagnosticPhases,
  diagnosticReference,
  diagnosticFinal,
} from "@/models/diagnostics";
import type { TranslateFn } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";
import styles from "./TurnDiagnostics.module.css";

type Props = {
  room: string;
  source?: string;
  turn?: string;
  agents?: AgentLike[];
  t: TranslateFn;
  onClose: () => void;
};

export function DiagnosticMessageAction({
  metadata,
  t,
  iconOnly = false,
}: {
  iconOnly?: boolean;
  metadata?: Record<string, unknown> | null;
  t: TranslateFn;
}) {
  const action = useRef<HTMLButtonElement>(null);
  const ref = diagnosticReference(metadata);
  const [open, setOpen] = useState(false);
  const room = ref?.room;
  const source = ref?.source;
  const turn = ref?.turn;
  const final = diagnosticFinal(metadata);
  useEffect(() => {
    if (!iconOnly || !room || !source || !turn) return;
    return observeDiagnosticRender(
      room,
      source,
      turn,
      final,
      action.current?.closest(".message-card, .thread-message-main"),
    );
  }, [room, source, turn, final, iconOnly]);
  if (!ref) return null;
  const meta = metadata?.csgclaw as Record<string, unknown> | null | undefined;
  return (
    <>
      {iconOnly ? (
        <button
          type="button"
          ref={action}
          className="message-action-button"
          aria-label={t("diagThisTurn")}
          data-tooltip={t("diagThisTurn")}
          data-tooltip-side="top"
          onClick={() => setOpen(true)}
        >
          <Activity aria-hidden="true" />
        </button>
      ) : meta?.runtime_error ? (
        <InlineDiagnosticError room={ref.room} source={ref.source} turn={ref.turn} t={t} onOpen={() => setOpen(true)} />
      ) : null}
      {open ? (
        <TurnDiagnostics
          key={ref.room + ref.source}
          room={ref.room}
          source={ref.source}
          turn={ref.turn}
          t={t}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </>
  );
}
function InlineDiagnosticError({
  room,
  source,
  turn,
  t,
  onOpen,
}: {
  room: string;
  source: string;
  turn?: string;
  t: TranslateFn;
  onOpen: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  return (
    <details className={styles.inlineError} onToggle={(event) => setExpanded(event.currentTarget.open)}>
      <summary>{t("diagExpandError")}</summary>
      {expanded ? <InlineDiagnosticErrorContent room={room} source={source} turn={turn} t={t} onOpen={onOpen} /> : null}
    </details>
  );
}
function InlineDiagnosticErrorContent({
  room,
  source,
  turn,
  t,
  onOpen,
}: {
  room: string;
  source: string;
  turn?: string;
  t: TranslateFn;
  onOpen: () => void;
}) {
  const [record, setRecord] = useState<TurnDiagnostic | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void fetchDiagnostics(room, { source_id: source }, controller.signal)
      .then(async (page) => {
        const item = page.items.find((item) => item.turn_id === turn) ?? page.items[0];
        if (!item) throw new Error("missing");
        return fetchDiagnostic(room, item.id, controller.signal);
      })
      .then((item) => {
        if (!controller.signal.aborted) setRecord(item);
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true);
      });
    return () => controller.abort();
  }, [room, source, turn]);
  return (
    <div>
      {record?.error ? (
        <>
          <strong>{record.error.code}</strong>
          <pre>{record.error.message}</pre>
        </>
      ) : (
        <p>{t(failed ? "diagEmpty" : "agentLogsLoading")}</p>
      )}
      <Button size="sm" variant="tertiaryGray" onClick={onOpen}>
        {t("diagThisTurn")}
      </Button>
    </div>
  );
}

export function DiagnosticHeaderAction({ room, agents = [], t }: Omit<Props, "onClose" | "source" | "turn">) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Tooltip content={t("diagConversation")} contentProps={{ side: "bottom", sideOffset: 6 }}>
        <Button
          className="icon-button"
          variant="secondaryGray"
          iconOnly
          size="lg"
          aria-label={t("diagConversation")}
          onClick={() => setOpen(true)}
        >
          <Activity size={18} />
        </Button>
      </Tooltip>
      {open ? <TurnDiagnostics key={room} room={room} agents={agents} t={t} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

export function TurnDiagnostics({ room, source = "", turn, agents = [], t, onClose }: Props) {
  const [items, setItems] = useState<TurnDiagnostic[]>([]);
  const [selected, setSelected] = useState("");
  const [detail, setDetail] = useState<TurnDiagnostic | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [agent, setAgent] = useState("");
  const [status, setStatus] = useState("");
  const [tab, setTab] = useState("turns");
  const [logs, setLogs] = useState("");
  const [cursor, setCursor] = useState("");
  const [nextCursor, setNextCursor] = useState("");
  const [revision, setRevision] = useState(0);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (tab !== "turns") return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let loadedSummary = "";
    const refresh = async () => {
      try {
        const response = await fetchDiagnostics(
          room,
          { source_id: source, agent_id: agent, status, cursor },
          controller.signal,
        );
        if (controller.signal.aborted) return;
        setItems(response.items);
        setNextCursor(response.next_cursor);
        const id =
          response.items.find((item) => item.id === selected)?.id ??
          response.items.find((item) => item.turn_id === turn)?.id ??
          response.items[0]?.id ??
          "";
        if (id !== selected) setSelected(id);
        const summaryKey = JSON.stringify(response.items.find((item) => item.id === id));
        if (id && summaryKey !== loadedSummary) {
          const record = await fetchDiagnostic(room, id, controller.signal);
          if (!controller.signal.aborted) {
            setDetail(record);
            loadedSummary = summaryKey;
          }
        } else if (!id) setDetail(null);
        setError("");
      } catch {
        if (!controller.signal.aborted) setError(t("diagLoadFailed"));
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          timer = setTimeout(refresh, 2000);
        }
      }
    };
    void refresh();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [room, source, selected, turn, agent, status, cursor, revision, tab, t]);

  const names = new Map(agents.map((item) => [item.id ?? "", item.name || item.id || ""]));
  const agentOptions = Array.from(new Set([...names.keys(), ...items.map((item) => item.agent_id)]))
    .filter(Boolean)
    .map((id) => ({ value: id, label: names.get(id) || items.find((item) => item.agent_id === id)?.agent_name || id }));
  const activeAgent = agent || detail?.agent_id || agentOptions[0]?.value || "";
  useEffect(() => {
    if (tab !== "logs" || !activeAgent) return;
    let live = true;
    setLogs("");
    void fetchAgentLogsRequest(activeAgent, { lines: 400 })
      .then((value) => {
        if (live) setLogs(value);
      })
      .catch(() => {
        if (live) setError(t("diagLoadFailed"));
      });
    return () => {
      live = false;
    };
  }, [tab, activeAgent, revision, t]);

  const exportJSON = async (download: boolean) => {
    if (!detail) return;
    const text = JSON.stringify(detail, null, 2);
    if (!download) {
      try {
        await navigator.clipboard.writeText(text);
        setCopied(true);
      } catch {
        setError(t("diagCopyFailed"));
      }
      return;
    }
    const url = URL.createObjectURL(new Blob([text], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = `turn-${detail.id}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <div>
            <DialogTitle>{t(source ? "diagThisTurn" : "diagConversation")}</DialogTitle>
            <DialogDescription>{t("diagDescription")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} iconOnly size="sm" variant="tertiaryGray" />
        </DialogHeader>
        <div className={styles.toolbar}>
          <div className={styles.tabs} role="group" aria-label={t("diagConversation")}>
            <Button
              size="sm"
              aria-pressed={tab === "turns"}
              variant={tab === "turns" ? "primary" : "secondaryGray"}
              onClick={() => setTab("turns")}
            >
              {t("diagTurns")}
            </Button>
            <Button
              size="sm"
              aria-pressed={tab === "logs"}
              variant={tab === "logs" ? "primary" : "secondaryGray"}
              onClick={() => setTab("logs")}
            >
              {t("agentLogsTitle")}
            </Button>
          </div>
          <Select
            size="sm"
            triggerClassName={styles.filter}
            triggerProps={{ "aria-label": t("diagAgent") }}
            value={agent}
            options={[{ value: "", label: t("diagAllAgents") }, ...agentOptions]}
            onValueChange={(value) => {
              setAgent(value);
              setCursor("");
              setSelected("");
              setDetail(null);
            }}
          />
          {tab === "turns" ? (
            <Select
              size="sm"
              triggerClassName={styles.filter}
              triggerProps={{ "aria-label": t("diagStatus") }}
              value={status}
              options={["", "running", "queued", "succeeded", "failed", "canceled", "interrupted"].map((value) => ({
                value,
                label: value ? t(`diagStatus_${value}`) : t("diagAllStatuses"),
              }))}
              onValueChange={(value) => {
                setStatus(value);
                setCursor("");
                setSelected("");
                setDetail(null);
              }}
            />
          ) : null}
          <Button
            iconOnly
            size="sm"
            variant="tertiaryGray"
            aria-label={t("refreshLogs")}
            onClick={() => setRevision((value) => value + 1)}
          >
            <RefreshCw size={15} />
          </Button>
        </div>
        <DialogBody className={styles.body}>
          {error ? (
            <div role="alert" className={styles.warning}>
              {error}
            </div>
          ) : null}
          {tab === "logs" ? (
            <pre className={styles.raw}>{logs || t("agentLogsEmpty")}</pre>
          ) : (
            <>
              <aside className={styles.list} aria-label={t("diagTurns")}>
                {items.map((item) => (
                  <button
                    type="button"
                    key={item.id}
                    aria-pressed={selected === item.id}
                    className={styles.listItem}
                    onClick={() => {
                      setSelected(item.id);
                      setDetail(null);
                      setCopied(false);
                    }}
                  >
                    <strong>{names.get(item.agent_id) || item.agent_name || item.agent_id}</strong>
                    <span>
                      {new Date(item.started_at).toLocaleTimeString()} · {diagnosticDuration(item.total_ms)}
                    </span>
                    <span className={styles.status} data-error={item.status === "failed"}>
                      {t(`diagStatus_${item.status}`)}
                    </span>
                  </button>
                ))}
                {!items.length ? <p className={styles.empty}>{t(loading ? "agentLogsLoading" : "diagEmpty")}</p> : null}
                {cursor ? (
                  <Button size="sm" variant="tertiaryGray" onClick={() => setCursor("")}>
                    {t("diagLatest")}
                  </Button>
                ) : null}
                {nextCursor ? (
                  <Button
                    size="sm"
                    variant="tertiaryGray"
                    onClick={() => {
                      setCursor(nextCursor);
                      setSelected("");
                      setDetail(null);
                    }}
                  >
                    {t("diagOlder")}
                  </Button>
                ) : null}
              </aside>
              <main className={styles.detail}>
                {detail ? (
                  <>
                    <DiagnosticDetail record={detail} t={t} />
                    <div className={styles.export}>
                      <Button size="sm" variant="tertiaryGray" onClick={() => void exportJSON(false)}>
                        <Copy size={14} />
                        {t(copied ? "diagCopied" : "diagCopy")}
                      </Button>
                      <Button size="sm" variant="tertiaryGray" onClick={() => void exportJSON(true)}>
                        <Download size={14} />
                        {t("diagExport")}
                      </Button>
                    </div>
                  </>
                ) : (
                  <p className={styles.empty}>{t(loading || items.length ? "agentLogsLoading" : "diagEmpty")}</p>
                )}
              </main>
            </>
          )}
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

function DiagnosticDetail({ record, t }: { record: TurnDiagnostic; t: TranslateFn }) {
  const [owner, setOwner] = useState("");
  const [hiddenCategories, setHiddenCategories] = useState<readonly DiagnosticHiddenCategory[]>(
    diagnosticDefaultHiddenCategories,
  );
  const hiddenStats = diagnosticHiddenStats(record, hiddenCategories, owner);
  const [selectedSpan, setSelectedSpan] = useState("");
  const spans = record.spans || [];
  const phases = diagnosticPhases(record);
  const total = Math.max(record.total_ms, 1);
  const selectedRow = selectedSpan
    ? diagnosticTimelineRows(record).find((row) => row.span.id === selectedSpan)
    : undefined;
  const selected = selectedRow?.span;
  const stageName = (name: string) =>
    name.startsWith("tool.") ? `${t("diagTool")} · ${name.slice(5)}` : t(`diagStage_${name.replaceAll(".", "_")}`);
  return (
    <>
      <div className={styles.titleRow}>
        <div>
          <span className={styles.eyebrow}>
            {record.runtime || t("diagRuntimeUnknown")} · {record.model || t("diagModelUnknown")}
          </span>
          <h3>
            {t("diagTotal")} <strong>{diagnosticDuration(record.total_ms)}</strong>
          </h3>
        </div>
        <span className={styles.status} data-error={record.status === "failed"}>
          {t(`diagStatus_${record.status}`)}
        </span>
      </div>
      {record.incomplete ? (
        <p className={styles.warning}>
          <AlertCircle size={15} />
          {t("diagIncomplete")}
        </p>
      ) : null}
      <div className={styles.metrics}>
        {phases.map((phase, index) => (
          <div key={phase.id} className={styles.metric} data-owner={index === 1 ? "runtime" : "csgclaw"}>
            <span>{t(phase.name)}</span>
            <strong>
              {diagnosticDuration(
                phase.start === undefined || phase.end === undefined ? undefined : phase.end - phase.start,
              )}
            </strong>
            <small>
              {phase.start !== undefined && phase.end !== undefined
                ? `${(((phase.end - phase.start) / total) * 100).toFixed(1)}%`
                : t("diagNotMeasured")}
            </small>
          </div>
        ))}
      </div>
      <DiagnosticBreakdown record={record} t={t} />
      <p className={styles.hint}>{t("diagInclusiveHint")}</p>
      <div className={styles.facts}>
        <span>
          <Clock size={13} />
          {t("diagCSGObserved")}: <b>{diagnosticDuration(diagnosticOccupied(spans, "csgclaw", total))}</b>
        </span>
        <span>
          {t("diagFirstOutput")}: <b>{diagnosticDuration(record.first_output_ms)}</b>
        </span>
        <span>
          {t("diagUserWait")}: <b>{diagnosticDuration(diagnosticOccupied(spans, "user", total))}</b>
        </span>
        {record.browser?.complete_ms !== undefined ? (
          <span>
            {t("diagBrowser")}: <b>{diagnosticDuration(record.browser.complete_ms)}</b>
          </span>
        ) : null}
      </div>
      <div className={styles.llm}>
        <span className={styles.owner} data-owner="llm">
          LLM
        </span>
        <span>
          {spans.some((span) => span.owner === "llm")
            ? t("diagLLMMeasured", {
                count: String(spans.filter((span) => span.owner === "llm").length),
                duration: diagnosticDuration(diagnosticOccupied(spans, "llm", total)),
              })
            : t("diagLLMUnavailable")}
        </span>
      </div>
      {record.error ? (
        <section className={styles.errorCard} role="alert">
          <h4>
            <AlertCircle size={16} />
            {t("diagError")}
          </h4>
          <p>
            {t("diagFailureStage")}:{" "}
            {record.error.stage === "delivery"
              ? t("diagAfter")
              : record.error.stage === "runtime"
                ? t("diagRuntime")
                : record.error.stage === "execution"
                  ? t("diagExecution")
                  : stageName(record.error.stage)}
          </p>
          <strong>{record.error.code}</strong>
          <pre>{record.error.message}</pre>
        </section>
      ) : null}
      <p className={styles.hint}>
        {t(spans.some((span) => span.details?.source === "codex_otel") ? "diagNativeHint" : "diagNativeMissing")}
      </p>
      <DiagnosticCalls record={record} t={t} />
      <div className={styles.timelineHeader}>
        <h4>{t("diagTimeline")}</h4>
        <div className={styles.timelineFilters}>
          <div className={styles.hiddenFilter}>
            <DropdownMenuRoot modal={false}>
              <DropdownMenuTrigger asChild>
                <Button size="sm" variant="secondaryGray" aria-label={t("diagHiddenEvents")}>
                  {t("diagHiddenEvents")} ({hiddenCategories.length})<ChevronDown size={14} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent className={styles.hiddenMenu} align="start" aria-label={t("diagHiddenEvents")}>
                <p className={styles.hiddenHelp}>{t("diagHiddenMenuHint")}</p>
                {hiddenStats.categories.map((category) => (
                  <DropdownMenuCheckboxItem
                    key={category.key}
                    checked={hiddenCategories.includes(category.key)}
                    aria-label={t(`diagHidden_${category.key}`)}
                    onSelect={(event) => event.preventDefault()}
                    onCheckedChange={(checked) =>
                      setHiddenCategories((current) =>
                        checked
                          ? [...current.filter((key) => key !== category.key), category.key]
                          : current.filter((key) => key !== category.key),
                      )
                    }
                  >
                    <span className={styles.hiddenCategory}>
                      <strong>{t(`diagHidden_${category.key}`)}</strong>
                      <span>
                        {diagnosticDuration(category.duration)} ·{" "}
                        {t("diagEventCount", { count: String(category.count) })}
                      </span>
                      <small>{t(`diagHidden_${category.key}_hint`)}</small>
                    </span>
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenuRoot>
            <TooltipProvider>
              <Tooltip
                content={t("diagHiddenSummaryHint")}
                contentProps={{ className: styles.spanTooltip, side: "top" }}
              >
                <span className={styles.hiddenCost} tabIndex={0} aria-live="polite" data-hidden-summary="">
                  {t("diagHiddenCount", { count: String(hiddenStats.count) })} ·{" "}
                  {diagnosticDuration(hiddenStats.duration)}
                </span>
              </Tooltip>
            </TooltipProvider>
            {hiddenStats.attention.count > 0 ? (
              <TooltipProvider>
                <Tooltip
                  content={t("diagHiddenAttentionHint", {
                    failed: String(hiddenStats.attention.failed),
                    slow: String(hiddenStats.attention.slow),
                  })}
                  contentProps={{ className: styles.spanTooltip, side: "top" }}
                >
                  <Button
                    size="sm"
                    variant="tertiaryGray"
                    className={styles.hiddenAttention}
                    onClick={() =>
                      setHiddenCategories((current) =>
                        current.filter((key) => !hiddenStats.attention.categories.includes(key)),
                      )
                    }
                  >
                    <AlertCircle size={14} />
                    {t("diagHiddenAttention", { count: String(hiddenStats.attention.count) })}
                  </Button>
                </Tooltip>
              </TooltipProvider>
            ) : null}
          </div>
          <Select
            size="sm"
            triggerClassName={styles.filter}
            triggerProps={{ "aria-label": t("diagOwner") }}
            value={owner}
            options={[
              { value: "", label: t("diagAllOwners") },
              ...["csgclaw", "runtime", "llm", "tool", "user"].map((value) => ({
                value,
                label: t(`diagOwner_${value}`),
              })),
            ]}
            onValueChange={setOwner}
          />
        </div>
      </div>
      <div className={styles.ruler}>
        <span>0 ms</span>
        <span>{diagnosticDuration(record.total_ms)}</span>
      </div>
      <DiagnosticTimeline
        record={record}
        hiddenCategories={hiddenCategories}
        owner={owner}
        selected={selectedSpan}
        onSelect={setSelectedSpan}
        t={t}
      />
      {selected ? (
        <section className={styles.spanDetail}>
          <h4>{diagnosticSpanLabel(selected, t, selectedRow?.callOrdinal)}</h4>
          <DiagnosticSpanFields span={selected} t={t} />
          <dl>
            <dt>{t("diagOwner")}</dt>
            <dd>{t(`diagOwner_${selected.owner}`)}</dd>
            <dt>{t("diagStartTime")}</dt>
            <dd>{diagnosticTimestamp(record, selected.start_ms)}</dd>
            <dt>{t("diagEndTime")}</dt>
            <dd>{diagnosticTimestamp(record, selected.end_ms)}</dd>
            <dt>{t("diagOffset")}</dt>
            <dd>
              {diagnosticDuration(selected.start_ms)} → {diagnosticDuration(selected.end_ms)}
            </dd>
            <dt>{t("diagStatus")}</dt>
            <dd>{t(`diagStatus_${selected.status}`)}</dd>
          </dl>
        </section>
      ) : null}
      <details className={styles.technical}>
        <summary>{t("diagTechnical")}</summary>
        <dl>
          <dt>Trace</dt>
          <dd>{record.id}</dd>
          <dt>Turn</dt>
          <dd>{record.turn_id}</dd>
          {record.runtime_session_id ? (
            <>
              <dt>Runtime session</dt>
              <dd>{record.runtime_session_id}</dd>
            </>
          ) : null}
          {record.runtime_turn_id ? (
            <>
              <dt>Runtime turn</dt>
              <dd>{record.runtime_turn_id}</dd>
            </>
          ) : null}
          {record.runtime_request_id ? (
            <>
              <dt>Runtime RPC</dt>
              <dd>{record.runtime_request_id}</dd>
            </>
          ) : null}
          <dt>Source</dt>
          <dd>{record.source_id}</dd>
        </dl>
      </details>
    </>
  );
}
