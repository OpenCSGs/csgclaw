import { X } from "lucide-react";
import { DialogClose } from "@/components/ui";
import styles from "./TasksView.module.css";

export function TaskDialogCloseButton({ label }: { label: string }) {
  return (
    <DialogClose asChild>
      <button type="button" className={styles.taskDialogCloseBtn} aria-label={label} title={label}>
        <X size={18} strokeWidth={1.75} aria-hidden="true" />
      </button>
    </DialogClose>
  );
}
