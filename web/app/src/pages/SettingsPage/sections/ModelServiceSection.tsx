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
    >
      <ModelProviderSettings embedded />
    </SettingsRow>
  );
}
