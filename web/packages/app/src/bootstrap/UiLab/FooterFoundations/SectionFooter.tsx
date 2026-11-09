import { useState } from "react";
import { ToggleGroup } from "radix-ui";
import { Button } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import desktopDivider from "./assets/divider-desktop.svg";
import mobileDivider from "./assets/divider-mobile.svg";
import plus from "./assets/plus.svg";
import styles from "./FooterFoundations.module.css";

export type FooterLayout = "Section" | "Card";
export type FooterBreakpoint = "Desktop" | "Mobile";
export type SectionFooterProps = {
  layout: FooterLayout;
  breakpoint: FooterBreakpoint;
  buttonGroup?: boolean;
  divider?: boolean;
  secondaryButton?: boolean;
  actions?: boolean;
  onAction: (action: string) => void;
};
export function SectionFooter({
  layout,
  breakpoint,
  buttonGroup = false,
  divider = true,
  secondaryButton = true,
  actions = true,
  onAction,
}: SectionFooterProps) {
  const { text: t } = useLabTranslation();
  const [period, setPeriod] = useState("12m");
  return (
    <footer className={styles.footer} data-layout={layout} data-breakpoint={breakpoint} aria-label={t("区块页脚示例")}>
      {divider && (
        <div className={styles.divider} aria-hidden="true">
          <img src={breakpoint === "Mobile" ? mobileDivider : desktopDivider} alt="" />
        </div>
      )}
      <div className={styles.content}>
        {buttonGroup && (
          <ToggleGroup.Root
            type="single"
            value={period}
            onValueChange={(value) => {
              if (value) {
                setPeriod(value);
                onAction({ "12m": "12 months", "30d": "30 days", "7d": "7 days", custom: "Custom" }[value] ?? value);
              }
            }}
            className={styles.periods}
            aria-label={t("时间范围")}
          >
            {["12m", "30d", "7d", "custom"].map((value, i) => (
              <ToggleGroup.Item
                key={value}
                value={value}
                className={styles.period}
                aria-label={t(["12 个月", "30 天", "7 天", "自定义范围"][i])}
              >
                {i === 3 && <img src={plus} alt="" />}
                <span className={styles.longLabel}>{t(["12 months", "30 days", "7 days", "Custom"][i])}</span>
                {i !== 3 && <span className={styles.shortLabel}>{value}</span>}
              </ToggleGroup.Item>
            ))}
          </ToggleGroup.Root>
        )}
        {secondaryButton && (
          <Button className={styles.learn} variant="linkGray" onClick={() => onAction("Learn more")}>
            {t("Learn more")}
          </Button>
        )}
        {actions && (
          <div className={styles.actions}>
            <Button className={styles.tertiary} variant="tertiaryGray" onClick={() => onAction("Tertiary")}>
              {t("Tertiary")}
            </Button>
            <Button className={styles.secondary} variant="secondaryGray" onClick={() => onAction("Secondary")}>
              {t("Secondary")}
            </Button>
            <Button className={styles.primary} variant="primary" onClick={() => onAction("Primary")}>
              {t("Primary")}
            </Button>
          </div>
        )}
      </div>
    </footer>
  );
}
