import { Plus } from "lucide-react";
import { Button } from "@/components/ui";
import { ModelProviderSettings } from "@/components/business/ModelProviderSettings";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function ModelServiceSection() {
  const controller = useWorkspaceControllerContext();
  const count = controller.modelProviders?.providers.length ?? 0;

  return (
    <SettingsRow
      title={
        <span>
          {controller.t("iaSettingsCardModels")}
          <span className={styles.countBadge}>{count}</span>
        </span>
      }
      description={controller.t("iaSettingsCardModelsHint")}
      actions={
        <Button variant="primary" size="md" onClick={() => void controller.sidebarProps?.onCreateModelProvider?.()}>
          <Plus size={14} strokeWidth={2.2} aria-hidden="true" />
          {controller.t("modelProviderAdd")}
        </Button>
      }
    >
      <ModelProviderSettings embedded />
    </SettingsRow>
  );
}
