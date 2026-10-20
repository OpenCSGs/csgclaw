import { useRef, useState } from "react";
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
  onOpenResourcePicker,
}: {
  disabled: boolean;
  t: TranslateFn;
  onAddFiles: () => void;
  onOpenResourcePicker?: (kind: TaskResourceKind) => void;
}) {
  const [open, setOpen] = useState(false);
  const openingResourcePicker = useRef(false);
  return (
    <PopoverRoot open={open} onOpenChange={setOpen}>
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
      <PopoverContent
        aria-label={t("composerAddContent")}
        className="composer-add-popover"
        role="dialog"
        side="top"
        onCloseAutoFocus={(event) => {
          if (openingResourcePicker.current) event.preventDefault();
          openingResourcePicker.current = false;
        }}
        onEscapeKeyDown={(event) => {
          if (isComposerKeyboardEventComposing(event)) event.preventDefault();
        }}
      >
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
            {resourceKinds.map(({ kind: resourceKind, icon: Icon, label: resourceLabel }) => (
              <button
                key={resourceKind}
                type="button"
                className="composer-add-menu-item"
                onClick={() => {
                  openingResourcePicker.current = true;
                  setOpen(false);
                  onOpenResourcePicker(resourceKind);
                }}
              >
                <Icon size={18} />
                {t(resourceLabel)}
              </button>
            ))}
          </section>
        ) : null}
      </PopoverContent>
    </PopoverRoot>
  );
}
