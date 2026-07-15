import { X } from "lucide-react";
import { DialogClose, Tooltip } from "@/components/ui";
import styles from "./TasksView.module.css";

export function TaskDialogCloseButton({ label }: { label: string }) {
  return (
    <Tooltip content={label}>
      <DialogClose asChild>
        <button type="button" className={styles.taskDialogCloseBtn} aria-label={label}>
          <X size={18} strokeWidth={1.75} aria-hidden="true" />
        </button>
      </DialogClose>
    </Tooltip>
  );
}
