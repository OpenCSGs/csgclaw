import { X } from "lucide-react";
import { IconButton } from "@/components/ui";

type ModalCloseButtonProps = {
  disabled?: boolean;
  label: string;
  onClose: () => void;
};

export function ModalCloseButton({ disabled = false, label, onClose }: ModalCloseButtonProps) {
  return (
    <IconButton
      className="modal-close"
      disabled={disabled}
      icon={<X size={20} strokeWidth={2} />}
      label={label}
      markClassName="modal-close-icon"
      onClick={onClose}
      variant="tertiaryGray"
    />
  );
}
