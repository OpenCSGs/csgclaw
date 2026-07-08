import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { IconButton } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import styles from "./SidebarRailControlButton.module.css";

type SidebarRailControlButtonProps = {
  className?: string;
  label: string;
  markClassName?: string;
  mode: "collapse" | "expand";
  onClick?: () => void;
};

export function SidebarRailControlButton({
  className,
  label,
  markClassName,
  mode,
  onClick,
}: SidebarRailControlButtonProps) {
  const Icon = mode === "expand" ? PanelLeftOpen : PanelLeftClose;

  return (
    <IconButton
      className={classNames(styles.button, className)}
      icon={<Icon size={20} strokeWidth={1.75} />}
      label={label}
      markClassName={classNames(styles.mark, markClassName)}
      onClick={onClick}
      size="md"
      title=""
      variant="tertiaryGray"
    />
  );
}
