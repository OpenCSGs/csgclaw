import type { SkillContinuation } from "@/models/slashCommands";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { TerminalIcon } from "lucide-react";
import { SidebarPuzzlePiece02Icon } from "@/components/ui/Icons";
import type { TranslateFn } from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";

export type SlashPickerProps = {
  continuation?: SkillContinuation;
  activeIndex?: number;
  candidates?: SlashPickerCandidate[];
  className?: string;
  loading?: boolean;
  onSelect: (name: string) => void;
  t: TranslateFn;
};

export function SlashPicker({
  continuation,
  candidates = [],
  activeIndex = 0,
  loading = false,
  className = "",
  t,
  onSelect,
}: SlashPickerProps) {
  const listRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const rowHeight = 44;
  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - 4);
  const end = Math.min(candidates.length, start + 24);
  useLayoutEffect(() => {
    if (listRef.current) listRef.current.scrollTop = 0;
    setScrollTop(0);
  }, [continuation?.scope]);
  useLayoutEffect(() => {
    const list = listRef.current;
    if (list && list.scrollTop > Math.max(0, candidates.length * rowHeight - list.clientHeight)) {
      list.scrollTop = Math.max(0, candidates.length * rowHeight - list.clientHeight);
      setScrollTop(list.scrollTop);
    }
  }, [candidates.length]);
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const top = activeIndex * rowHeight;
    if (top < list.scrollTop) list.scrollTop = top;
    else if (top + rowHeight > list.scrollTop + list.clientHeight) list.scrollTop = top + rowHeight - list.clientHeight;
    setScrollTop(list.scrollTop);
  }, [activeIndex]);
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (
      !sentinel ||
      !continuation?.hasMore ||
      continuation.loading ||
      continuation.failed ||
      typeof IntersectionObserver === "undefined"
    )
      return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void continuation.loadMore();
      },
      { root: listRef.current, rootMargin: "88px" },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [continuation]);

  return (
    <div
      ref={listRef}
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      className={`mention-picker slash-picker ${className}`.trim()}
      role="listbox"
    >
      {loading ? <div className="slash-picker-empty">{t("slashPickerLoading")}</div> : null}
      {!loading && !continuation?.failed && candidates.length === 0 ? (
        <div className="slash-picker-empty">{t("slashPickerEmpty")}</div>
      ) : null}
      <div aria-hidden="true" style={{ height: start * rowHeight, flexShrink: 0 }} />
      {candidates.slice(start, end).map((candidate, visibleIndex) => (
        <button
          key={`${candidate.type}:${candidate.name}`}
          style={{ height: rowHeight, minHeight: rowHeight, flexShrink: 0 }}
          aria-posinset={visibleIndex + start + 1}
          aria-setsize={continuation?.hasMore ? -1 : candidates.length}
          role="option"
          aria-selected={visibleIndex + start === activeIndex}
          className={`mention-option slash-option ${candidate.type === "command" ? "command-option" : "skill-slash-option"} ${visibleIndex + start === activeIndex ? "active" : ""}`}
          onMouseDown={(event) => {
            event.preventDefault();
            onSelect(candidate.name);
          }}
        >
          <span className="slash-option-mark" aria-hidden="true">
            {candidate.type === "command" ? (
              <TerminalIcon size={18} strokeWidth={1.8} />
            ) : (
              <SidebarPuzzlePiece02Icon size={18} />
            )}
          </span>
          <div className="slash-option-copy">
            <span className="message-author">{candidate.name}</span>
            {candidate.description ? <span className="slash-option-description">{candidate.description}</span> : null}
          </div>
          <span className="slash-option-kind">
            {candidate.type === "command" ? t("slashPickerCommandKind") : t("slashPickerSkillKind")}
          </span>
        </button>
      ))}
      <div aria-hidden="true" style={{ height: Math.max(0, candidates.length - end) * rowHeight, flexShrink: 0 }} />
      <div ref={sentinelRef}>
        {continuation?.failed ? (
          <div role="alert">
            {t("resourceLoadFailed")}{" "}
            <button type="button" onClick={continuation.retry}>
              {t("retry")}
            </button>
          </div>
        ) : null}
        {continuation?.loading && !loading ? <div role="status">{t("slashPickerLoading")}</div> : null}
        {continuation?.hasMore && !continuation.failed ? (
          <button type="button" disabled={continuation.loading} onClick={() => void continuation.loadMore()}>
            {t("resourceLoadMore")}
          </button>
        ) : null}
      </div>
    </div>
  );
}
