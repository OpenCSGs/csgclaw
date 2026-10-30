import { useLabTranslation } from "../LabLocale";
import { Button, IconButton } from "@/components/ui";
import { TooltipRoot, TooltipTrigger, TooltipContent, TooltipArrow } from "@/components/ui/Tooltip";
import { TooltipBody } from "./TooltipBubble";
import { tooltipPositions, arrowAssets, defaultTooltipTitle, defaultTooltipDescription } from "./tooltipData";
import helpClosed from "./assets/e9c84.svg";
import helpOpen from "./assets/8a184.svg";
import styles from "./TooltipFoundations.module.css";

type TooltipPreviewProps = {
  arrow: string;
  supporting: boolean;
  title: string;
  description: string;
  edge?: "left" | "right";
};

export function TooltipPreview({ arrow, supporting, title, description, edge }: TooltipPreviewProps) {
  const { text: labText } = useLabTranslation();

  const position = tooltipPositions.find((item) => item.value === arrow) ?? tooltipPositions[0];
  const side = edge ?? position.side;
  const hasArrow = arrow !== "None";
  const offCenter = arrow === "Bottom left" || arrow === "Bottom right";
  return (
    <TooltipRoot>
      <TooltipTrigger asChild>
        {edge ? (
          <IconButton
            className={styles.helpButton}
            size="sm"
            label={edge === "left" ? labText("左边缘提示") : labText("右边缘提示")}
            icon={
              <span className={styles.helpGlyph}>
                <img className={styles.closedIcon} src={helpClosed} alt="" />
                <img className={styles.openIcon} src={helpOpen} alt="" />
              </span>
            }
          />
        ) : (
          <Button size="sm">{labText("悬浮或聚焦查看提示")}</Button>
        )}
      </TooltipTrigger>
      <TooltipContent
        className={styles.liveContent}
        data-supporting={supporting}
        data-arrow={arrow}
        data-off-center={offCenter && !edge}
        side={side}
        align={edge ? "center" : position.align}
        sideOffset={4}
        arrowPadding={8}
        collisionPadding={12}
        aria-label={[title || defaultTooltipTitle, supporting ? description || defaultTooltipDescription : ""]
          .filter(Boolean)
          .join(" ")}
      >
        <TooltipBody supporting={supporting} title={title} description={description} />
        {hasArrow && (
          <TooltipArrow asChild width={offCenter && !edge ? 28 : 16} height={6}>
            <span className={styles.liveArrow} aria-hidden="true">
              <img data-arrow-side="top" className={styles.centerArrow} src={arrowAssets["Bottom center"]} alt="" />
              <img data-arrow-side="top" className={styles.startArrow} src={arrowAssets["Bottom left"]} alt="" />
              <img data-arrow-side="top" className={styles.endArrow} src={arrowAssets["Bottom right"]} alt="" />
              <img data-arrow-side="bottom" src={arrowAssets["Top center"]} alt="" />
              <img data-arrow-side="right" src={arrowAssets.Left} alt="" />
              <img data-arrow-side="left" src={arrowAssets.Right} alt="" />
            </span>
          </TooltipArrow>
        )}
      </TooltipContent>
    </TooltipRoot>
  );
}
