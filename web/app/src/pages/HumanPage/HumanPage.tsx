import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { HumanDetailPane } from "./components";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";
import { DialogRoot, DialogContent, DialogHeader, DialogTitle, DialogCloseButton } from "@/components/ui";
import { useState } from "react";

export function HumanPage() {
  const controller = useWorkspaceControllerContext();
  const { current, close } = useWorkspaceDialogs();
  const [avatarPortalContainer, setAvatarPortalContainer] = useState<HTMLDivElement | null>(null);

  if (!controller.ready) {
    return null;
  }

  const content = <HumanDetailPane {...controller.humanViewProps} avatarPortalContainer={avatarPortalContainer} />;
  if (current !== "profile") return content;
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent
        ref={setAvatarPortalContainer}
        style={{ width: "min(900px, calc(100vw - 48px))", padding: 0 }}
        aria-describedby={undefined}
        onEscapeKeyDown={(event) => {
          if (avatarPortalContainer?.querySelector('[role="dialog"][aria-modal="true"]')) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{controller.t("humanSection")}</DialogTitle>
          <DialogCloseButton label={controller.t("close")} />
        </DialogHeader>
        {content}
      </DialogContent>
    </DialogRoot>
  );
}
