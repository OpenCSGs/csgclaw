import { LinkButton } from "@/components/ui";
import { useContext, type ReactNode } from "react";
import { useLabTranslation } from "./LabLocale";
import { LabView } from "./labViewContext";
import styles from "./UiLab.module.css";

export function LabComponentHeading({
  sectionId,
  titleId,
  children,
  designAction,
}: {
  sectionId: string;
  titleId: string;
  children: ReactNode;
  designAction?: ReactNode;
}) {
  const view = useContext(LabView);
  const { text: t } = useLabTranslation();
  return (
    <div className={styles.componentHeading}>
      <h2 id={titleId}>{children}</h2>
      {view === "preview" ? (
        <LinkButton className={styles.designLink} variant="linkGray" href={`#${sectionId}/design`}>
          {t("设计详情")} →
        </LinkButton>
      ) : (
        designAction
      )}
    </div>
  );
}
