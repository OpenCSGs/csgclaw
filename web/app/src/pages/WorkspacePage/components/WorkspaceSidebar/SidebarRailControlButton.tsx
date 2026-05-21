import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { Button } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";

type SidebarRailControlButtonProps = {
  label: string;
  mode: "collapse" | "expand";
  onClick?: () => void;
};

export function SidebarRailControlButton({ label, mode, onClick }: SidebarRailControlButtonProps) {
  const Icon = mode === "expand" ? PanelLeftOpen : PanelLeftClose;

  return (
    <Button
      variant="ghost"
      className={classNames("sidebar-rail-control-button", mode === "expand" && "is-expand")}
      aria-label={label}
      title={label}
      onClick={onClick}
    >
      <span className="sidebar-rail-control-mark" aria-hidden="true">
        <Icon size={22} strokeWidth={2} />
      </span>
    </Button>
  );
}
