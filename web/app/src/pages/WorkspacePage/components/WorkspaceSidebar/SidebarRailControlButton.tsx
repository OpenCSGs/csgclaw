import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { IconButton } from "@/components/ui";

type SidebarRailControlButtonProps = {
  label: string;
  mode: "collapse" | "expand";
  onClick?: () => void;
};

export function SidebarRailControlButton({ label, mode, onClick }: SidebarRailControlButtonProps) {
  const Icon = mode === "expand" ? PanelLeftOpen : PanelLeftClose;

  return (
    <IconButton
      className="sidebar-rail-control-button"
      icon={<Icon size={20} strokeWidth={2} />}
      label={label}
      markClassName="sidebar-rail-control-mark"
      onClick={onClick}
      size="md"
      variant="tertiaryGray"
    />
  );
}
