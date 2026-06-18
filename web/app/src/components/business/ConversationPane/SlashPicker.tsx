import { useLayoutEffect, useRef } from "react";
import { BoxIcon, TerminalIcon } from "lucide-react";
import type { TranslateFn } from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";

export type SlashPickerProps = {
  activeIndex?: number;
  candidates?: SlashPickerCandidate[];
  className?: string;
  loading?: boolean;
  onSelect: (name: string) => void;
  t: TranslateFn;
};

export function SlashPicker({
  candidates = [],
  activeIndex = 0,
  loading = false,
  className = "",
  t,
  onSelect,
}: SlashPickerProps) {
  const activeOptionRef = useRef<HTMLButtonElement | null>(null);
  const activeCandidate = candidates[activeIndex] || null;
  const activeCandidateKey = activeCandidate ? `${activeCandidate.type}:${activeCandidate.name}` : "";

  useLayoutEffect(() => {
    activeOptionRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, activeCandidateKey, candidates.length]);

  return (
    <div className={`mention-picker slash-picker ${className}`.trim()} role="listbox">
      {loading ? <div className="slash-picker-empty">{t("slashPickerLoading")}</div> : null}
      {!loading && candidates.length === 0 ? <div className="slash-picker-empty">{t("slashPickerEmpty")}</div> : null}
      {candidates.map((candidate, index) => (
        <button
          key={`${candidate.type}:${candidate.name}`}
          ref={index === activeIndex ? activeOptionRef : null}
          role="option"
          aria-selected={index === activeIndex}
          className={`mention-option slash-option ${candidate.type === "command" ? "command-option" : "skill-slash-option"} ${index === activeIndex ? "active" : ""}`}
          onMouseDown={(event) => {
            event.preventDefault();
            onSelect(candidate.name);
          }}
        >
          <span className="slash-option-mark" aria-hidden="true">
            {candidate.type === "command" ? (
              <TerminalIcon size={18} strokeWidth={1.8} />
            ) : (
              <BoxIcon size={18} strokeWidth={1.8} />
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
    </div>
  );
}
