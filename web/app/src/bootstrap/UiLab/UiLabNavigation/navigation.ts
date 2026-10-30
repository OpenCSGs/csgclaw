export const labGroups = [
  {
    label: "设计基础",
    items: [
      { id: "design-foundations", label: "Colors · 颜色", zh: "颜色" },
      { id: "typography-foundations", label: "Typography · 排版", zh: "排版" },
      { id: "shadow-foundations", label: "Shadows · 阴影", zh: "阴影" },
      { id: "layout-foundations", label: "Spacing, radius & grids", zh: "间距、圆角与网格" },
    ],
  },
  {
    label: "设计组件",
    items: [
      { id: "button-foundations", label: "Buttons · 按钮", zh: "按钮" },
      { id: "tag-foundations", label: "Tags · 标签", zh: "标签" },
      { id: "input-foundations", label: "Input fields · 输入框", zh: "输入框" },
      { id: "dropdown-foundations", label: "Dropdowns · 下拉菜单", zh: "下拉菜单" },
      { id: "toggle-foundations", label: "Toggles · 开关", zh: "开关" },
      { id: "checkbox-foundations", label: "Checkboxes · 复选框", zh: "复选框" },
      { id: "slider-foundations", label: "Sliders · 滑块", zh: "滑块" },
      { id: "tooltip-foundations", label: "Tooltips · 气泡提示", zh: "气泡提示" },
      { id: "progress-foundations", label: "Progress indicators · 进度指示器", zh: "进度指示器" },
    ],
  },
  {
    label: "交互示例",
    items: [
      { id: "token-consumption", label: "Token 用量" },
      { id: "secondary-gray", label: "Secondary gray 按钮" },
      { id: "buttons-and-fields", label: "按钮与表单" },
      { id: "workspace-boundary", label: "工作区样式边界" },
      { id: "selection-controls", label: "选择控件" },
      { id: "reasoning-effort", label: "推理强度" },
      { id: "dialogs-and-empty", label: "弹窗与空状态" },
    ],
  },
] as const;

export type LabSectionId = (typeof labGroups)[number]["items"][number]["id"];

export function sectionFromHash(hash: string): LabSectionId {
  const id = hash.replace(/^#/, "");
  return labGroups.flatMap((group) => [...group.items]).find((item) => item.id === id)?.id ?? "design-foundations";
}
