import type { RefObject } from "react";
import { Activity, Folder, Globe, PanelRight } from "lucide-react";
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
      icon: Folder,
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

export function ChatSideToggle({
  controller,
  onCollapse,
  triggerRef,
  t,
}: {
  controller: ReturnType<typeof useChatSidePanel>;
  onCollapse: () => void;
  triggerRef: RefObject<HTMLElement | null>;
  t: TranslateFn;
}) {
  return (
    <Tooltip
      content={t(controller.state.open ? "chatSideCollapse" : "chatSideExpand")}
      contentProps={{ side: "bottom" }}
    >
      <Button
        className="chat-side-toggle"
        iconOnly
        size="lg"
        variant="secondaryGray"
        aria-label={t(controller.state.open ? "chatSideCollapse" : "chatSideExpand")}
        aria-expanded={controller.state.open}
        onClick={(event) => {
          triggerRef.current = event.currentTarget;
          if (controller.state.open) onCollapse();
          else controller.patch({ open: true });
        }}
      >
        <PanelRight size={19} />
      </Button>
    </Tooltip>
  );
}
