import { useEffect, useState } from "react";
import { Bot, Image as ImageIcon, Video } from "lucide-react";
import { fetchAgentProfileDefaults } from "@/api/agents";
import { errorMessage } from "@/api/client";
import { TextInput } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import type { AgentProfileLike } from "@/models/agents";
import styles from "./ModelDefaultsPanel.module.css";

export function ModelDefaultsPanel() {
  const { t } = useWorkspaceControllerContext();
  const [defaults, setDefaults] = useState<AgentProfileLike | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let cancelled = false;
    void fetchAgentProfileDefaults()
      .then((value) => {
        if (!cancelled) setDefaults(value);
      })
      .catch((cause) => {
        if (!cancelled) setError(errorMessage(cause, t("profileLoadFailed")));
      });
    return () => {
      cancelled = true;
    };
  }, [t]);
  const rows = [
    { key: "modelDefaultsAgent", icon: Bot, value: defaults?.model_id },
    { key: "modelDefaultsImage", icon: ImageIcon, value: defaults?.image_generation?.model_id },
    { key: "modelDefaultsVideo", icon: Video, value: defaults?.video_generation?.model_id },
  ];
  return (
    <section className={styles.panel} aria-label={t("modelDefaultsTitle")}>
      <header className={styles.head}>
        <h3 className={styles.title}>{t("modelDefaultsTitle")}</h3>
        <p className={styles.hint}>{t("modelDefaultsReadonlyHint")}</p>
      </header>
      {error ? (
        <p role="alert">{error}</p>
      ) : (
        <div className={styles.rows}>
          {rows.map(({ key, icon: Icon, value }) => (
            <label key={key} className={styles.row}>
              <span className={styles.rowLabel}>
                <Icon size={14} />
                {t(key)}
              </span>
              <TextInput readOnly aria-label={t(key)} value={value || t("modelDefaultsNone")} />
            </label>
          ))}
        </div>
      )}
    </section>
  );
}
