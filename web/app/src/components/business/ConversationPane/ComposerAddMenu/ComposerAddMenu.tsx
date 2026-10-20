import { Plus, Paperclip, Wrench, Boxes, BookOpen } from "lucide-react";
import { Button, PopoverClose, PopoverContent, PopoverRoot, PopoverTrigger, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
export type TaskResourceKind = "skill" | "connector" | "knowledge";
export function ComposerAddMenu({
  disabled,
  t,
  onAddFiles,
  onOpenResourcePicker,
}: {
  disabled: boolean;
  t: TranslateFn;
  onAddFiles: () => void;
  onOpenResourcePicker?: (kind: TaskResourceKind) => void;
}) {
  return (
    <PopoverRoot>
      <Tooltip content={t("composerAddContent")} contentProps={{ side: "top" }}>
        <PopoverTrigger asChild>
          <Button
            aria-label={t("composerAddContent")}
            className="composer-add-button"
            disabled={disabled}
            iconOnly
            size="sm"
            variant="tertiaryGray"
          >
            <Plus size={18} />
          </Button>
        </PopoverTrigger>
      </Tooltip>
      <PopoverContent aria-label={t("composerAddContent")} className="composer-add-popover" role="dialog" side="top">
        <section className="composer-add-section">
          <PopoverClose asChild>
            <button type="button" className="composer-add-menu-item" onClick={onAddFiles}>
              <Paperclip size={18} />
              {t("addAttachment")}
            </button>
          </PopoverClose>
        </section>
        {onOpenResourcePicker ? (
          <section className="composer-add-section">
            <div className="composer-add-section-label">{t("composerAddResources")}</div>
            {(
              [
                { kind: "skill", icon: Wrench, label: "composerResourceSkill" },
                { kind: "connector", icon: Boxes, label: "composerResourceConnector" },
                { kind: "knowledge", icon: BookOpen, label: "composerResourceKnowledge" },
              ] as const
            ).map(({ kind, icon: Icon, label }) => (
              <PopoverClose key={kind} asChild>
                <button type="button" className="composer-add-menu-item" onClick={() => onOpenResourcePicker(kind)}>
                  <Icon size={18} />
                  {t(label)}
                </button>
              </PopoverClose>
            ))}
          </section>
        ) : null}
      </PopoverContent>
    </PopoverRoot>
  );
}
