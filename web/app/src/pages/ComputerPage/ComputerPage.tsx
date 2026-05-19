import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ComputerView } from "./components";

export function ComputerPage() {
  const controller = useWorkspaceControllerContext();

  if (!controller.ready) {
    return null;
  }

  return <ComputerView {...controller.computerViewProps} />;
}
