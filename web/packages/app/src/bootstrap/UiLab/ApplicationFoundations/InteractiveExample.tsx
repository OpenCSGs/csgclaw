import { Fragment, useState, type ReactNode } from "react";
import { Button } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import canvasStyles from "../LabPreviewCanvas.module.css";
import styles from "./ApplicationFoundations.module.css";

export function InteractiveExample({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  const { text: t } = useLabTranslation();
  const [revision, setRevision] = useState(0);

  return (
    <section id={id} className={styles.foundation} aria-labelledby={`${id}-preview-title`}>
      <h2 id={`${id}-preview-title`}>{title}</h2>
      <p>{description}</p>
      <section
        className={`${canvasStyles.previewCanvas} ${styles.liveStage} ${styles.demoSurface} ${styles.withReset}`}
        aria-label={`${title} · ${t("交互试用")}`}
      >
        <div className={styles.canvasReset}>
          <Button size="sm" onClick={() => setRevision((value) => value + 1)}>
            {t("重置示例")}
          </Button>
        </div>
        <Fragment key={revision}>{children}</Fragment>
      </section>
    </section>
  );
}
