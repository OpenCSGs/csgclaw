import type { RefObject } from "react";
import { Activity, Globe } from "lucide-react";
import { FolderIcon } from "@/components/business/FileTypeIcon";
import { Button, Tooltip } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { useChatSidePanel } from "./useChatSidePanel";

export function ChatToolsToolbar({
  controller,
  onActivity,
  triggerRef,
  t,
}: {
  controller: ReturnType<typeof useChatSidePanel>;
  onActivity: () => void;
  triggerRef: RefObject<HTMLElement | null>;
  t: TranslateFn;
}) {
  const actions = [
    { id: "activity", label: t("conversationActivityOpen"), icon: Activity, action: onActivity },
    {
      id: "files",
      label: t("chatFiles"),
      icon: FolderIcon,
      action: () => controller.openTab({ id: "files", kind: "files" }),
    },
    {
      id: "browser",
      label: t("chatBrowser"),
      icon: Globe,
      action: () => {
        const browser = controller.state.tabs.find((tab) => tab.kind === "browser");
        if (browser) controller.patch({ open: true, activeID: browser.id });
        else controller.openTab({ id: crypto.randomUUID(), kind: "browser", url: "" });
      },
    },
  ];
  return (
    <div className="chat-tools-actions">
      {actions.map(({ id, label, icon: Icon, action }) => (
        <Tooltip key={id} content={label} contentProps={{ side: "bottom" }}>
          <Button
            iconOnly
            size="lg"
            variant="secondaryGray"
            active={
              controller.state.open &&
              controller.state.tabs.find((tab) => tab.id === controller.state.activeID)?.kind === id
            }
            aria-label={label}
            aria-pressed={
              controller.state.open &&
              (controller.state.activeID === id ||
                (id === "browser" &&
                  controller.state.tabs.find((tab) => tab.id === controller.state.activeID)?.kind === "browser"))
            }
            onClick={(event) => {
              triggerRef.current = event.currentTarget;
              action();
            }}
          >
            <Icon size={19} />
          </Button>
        </Tooltip>
      ))}
    </div>
  );
}
