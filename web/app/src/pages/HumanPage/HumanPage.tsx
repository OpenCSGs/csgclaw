import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { HumanDetailPane } from "./components";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";
import { DialogRoot, DialogContent, DialogHeader, DialogTitle, DialogCloseButton } from "@/components/ui";

export function HumanPage() {
  const controller = useWorkspaceControllerContext();
  const { current, close } = useWorkspaceDialogs();

  if (!controller.ready) {
    return null;
  }

  const content = <HumanDetailPane {...controller.humanViewProps} />;
  if (current !== "profile") return content;
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent style={{ width: "min(900px, calc(100vw - 48px))", padding: 0 }} aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{controller.t("humanSection")}</DialogTitle>
          <DialogCloseButton label={controller.t("close")} />
        </DialogHeader>
        {content}
      </DialogContent>
    </DialogRoot>
  );
}
