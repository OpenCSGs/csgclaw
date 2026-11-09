import bottom from "./assets/e63be.svg";
import top from "./assets/2452d.svg";
import bottomLeft from "./assets/28b38.svg";
import bottomRight from "./assets/9bdc9.svg";
import left from "./assets/c1c34.svg";
import right from "./assets/722bb.svg";
export const defaultTooltipTitle = "This is a tooltip";
export const defaultTooltipDescription =
  "Tooltips are used to describe or identify an  element. In most scenarios, tooltips help the user understand meaning, function or alt-text.";
export const tooltipPositions = [
  { value: "None", label: "无箭头", side: "top", align: "center" },
  { value: "Bottom center", label: "下居中", side: "top", align: "center" },
  { value: "Bottom left", label: "下偏左", side: "top", align: "start" },
  { value: "Bottom right", label: "下偏右", side: "top", align: "end" },
  { value: "Top center", label: "上居中", side: "bottom", align: "center" },
  { value: "Left", label: "左侧", side: "right", align: "center" },
  { value: "Right", label: "右侧", side: "left", align: "center" },
] as const;
export const arrowAssets: Record<string, string> = {
  "Bottom center": bottom,
  "Top center": top,
  "Bottom left": bottomLeft,
  "Bottom right": bottomRight,
  Left: left,
  Right: right,
};
