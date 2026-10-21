import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import {
  Button,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { MAX_FEEDBACK_DESCRIPTION_LENGTH, MAX_FEEDBACK_IMAGES } from "@/models/feedback";
import styles from "./FeedbackDialog.module.css";

type Props = {
  open: boolean;
  busy: boolean;
  description: string;
  images: File[];
  error: string;
  t: TranslateFn;
  onOpenChange: (open: boolean) => void;
  onDescriptionChange: (value: string) => void;
  onImagesChange: (value: File[]) => void;
  onSubmit: () => void;
};

export function FeedbackDialog(props: Props) {
  const { t, images, busy } = props;
  const imageInputRef = useRef<HTMLInputElement>(null);
  const imageLimitReached = images.length >= MAX_FEEDBACK_IMAGES;
  return (
    <DialogRoot
      open={props.open}
      onOpenChange={(open) => {
        if (!busy) props.onOpenChange(open);
      }}
    >
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <DialogTitle>{t("feedbackTitle")}</DialogTitle>
          <DialogDescription>{t("settingsFeedbackDescription")}</DialogDescription>
        </DialogHeader>
        <DialogBody className={styles.body}>
          <label className={styles.label}>
            {t("settingsFeedbackLabel")}
            <textarea
              value={props.description}
              disabled={busy}
              rows={6}
              maxLength={MAX_FEEDBACK_DESCRIPTION_LENGTH}
              placeholder={t("settingsFeedbackPlaceholder")}
              onChange={(event) => props.onDescriptionChange(event.target.value)}
            />
          </label>
          <div className={styles.label}>
            <span>{t("feedbackImages")} ({images.length}/{MAX_FEEDBACK_IMAGES})</span>
            <input
              ref={imageInputRef}
              className={styles.fileInput}
              type="file"
              accept="image/jpeg,image/png"
              multiple
              disabled={busy}
              tabIndex={-1}
              aria-hidden="true"
              onChange={(event) => {
                props.onImagesChange([...images, ...Array.from(event.target.files || [])]);
                event.target.value = "";
              }}
            />
            <Tooltip
              content={t(imageLimitReached ? "feedbackImageLimitReached" : "feedbackImageLimit")}
              contentProps={{ side: "top" }}
            >
              <span className={styles.chooseFilesTooltipTarget}>
                <Button
                  variant="secondaryGray"
                  size="sm"
                  className={styles.chooseFilesButton}
                  disabled={busy || imageLimitReached}
                  onClick={() => imageInputRef.current?.click()}
                >
                  {t("feedbackChooseImages")}
                </Button>
              </span>
            </Tooltip>
            <p className={styles.imageHint}>{t("feedbackImageLimit")}</p>
          </div>
          <div className={styles.images}>
            {images.map((file, index) => (
              <div className={styles.image} key={`${index}-${file.name}`}>
                <Preview file={file} />
                <Button
                  iconOnly
                  variant="secondaryGray"
                  size="sm"
                  className={styles.removeImage}
                  disabled={busy}
                  aria-label={`${t("feedbackRemoveImage")} ${index + 1}`}
                  onClick={() => props.onImagesChange(images.filter((_, position) => position !== index))}
                >
                  <X size={14} strokeWidth={2} aria-hidden="true" />
                </Button>
              </div>
            ))}
          </div>
          {props.error ? <p role="alert">{props.error}</p> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => props.onOpenChange(false)}>
            {t("close")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={busy}
            disabled={busy || (!props.description.trim() && !images.length)}
            onClick={props.onSubmit}
          >
            {t("feedbackSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

function Preview({ file }: { file: File }) {
  const [url, setURL] = useState("");
  useEffect(() => {
    const next = URL.createObjectURL(file);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return <img src={url || undefined} alt={file.name} />;
}
