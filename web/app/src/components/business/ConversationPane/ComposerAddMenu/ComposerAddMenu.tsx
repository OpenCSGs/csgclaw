import { useState, type ReactNode } from "react";
import { Plus, Paperclip, Wrench, Boxes, BookOpen } from "lucide-react";
import { Button, PopoverClose, PopoverContent, PopoverRoot, PopoverTrigger, Tooltip } from "@/components/ui";
import { isComposerKeyboardEventComposing } from "@/models/composer";
import type { TranslateFn } from "@/models/conversations";
export type TaskResourceKind = "skill" | "connector" | "knowledge";

const resourceKinds = [
  { kind: "skill", icon: Wrench, label: "composerResourceSkill" },
  { kind: "connector", icon: Boxes, label: "composerResourceConnector" },
  { kind: "knowledge", icon: BookOpen, label: "composerResourceKnowledge" },
] as const;

export function ComposerAddMenu({
  disabled,
  t,
  onAddFiles,
  renderResourcePicker,
}: {
  disabled: boolean;
  t: TranslateFn;
  onAddFiles: () => void;
  renderResourcePicker?: (kind: TaskResourceKind, close: () => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<TaskResourceKind | null>(null);
  function changeOpen(value: boolean) {
    setOpen(value);
    if (!value) setKind(null);
  }
  const label = resourceKinds.find((item) => item.kind === kind)?.label ?? "composerAddContent";
  return (
    <PopoverRoot open={open} onOpenChange={changeOpen}>
      <Tooltip content={t("composerAddContent")}>
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
      <PopoverContent
        aria-label={t(label)}
        className={kind ? undefined : "composer-add-popover"}
        role="dialog"
        side="top"
        onEscapeKeyDown={(event) => {
          if (isComposerKeyboardEventComposing(event)) event.preventDefault();
        }}
      >
        {kind && renderResourcePicker ? (
          renderResourcePicker(kind, () => changeOpen(false))
        ) : (
          <>
            <section className="composer-add-section">
              <PopoverClose asChild>
                <button type="button" className="composer-add-menu-item" onClick={onAddFiles}>
                  <Paperclip size={18} />
                  {t("addAttachment")}
                </button>
              </PopoverClose>
            </section>
            {renderResourcePicker ? (
              <section className="composer-add-section">
                <div className="composer-add-section-label">{t("composerAddResources")}</div>
                {resourceKinds.map(({ kind: resourceKind, icon: Icon, label: resourceLabel }) => (
                  <button
                    key={resourceKind}
                    type="button"
                    className="composer-add-menu-item"
                    onClick={() => setKind(resourceKind)}
                  >
                    <Icon size={18} />
                    {t(resourceLabel)}
                  </button>
                ))}
              </section>
            ) : null}
          </>
        )}
      </PopoverContent>
    </PopoverRoot>
  );
}
