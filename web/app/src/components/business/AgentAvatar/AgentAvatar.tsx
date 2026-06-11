import { AgentIcon } from "@/components/ui/Icons";
import { normalizeAvatarPath } from "@/shared/avatar";
import { Edit3, ImagePlus, UploadCloud, X, ZoomIn, ZoomOut } from "lucide-react";
import type { ChangeEvent, DragEvent, PointerEvent as ReactPointerEvent, ReactNode } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

const AVATAR_GROUPS = [
  { key: "3D", labelKey: "agentAvatarStyle3D" },
  { key: "cartoon", labelKey: "agentAvatarStyleCartoon" },
  { key: "pic", labelKey: "agentAvatarStylePic" },
] as const;

export const AGENT_AVATAR_OPTIONS = AVATAR_GROUPS.flatMap((group) =>
  Array.from({ length: 8 }, (_, index) => ({
    group: group.key,
    labelKey: group.labelKey,
    index: index + 1,
    value: `avatar/${group.key}-${index + 1}.png`,
  })),
);

type TranslateFn = (key: string) => string;
type AvatarEditorTab = "builtin" | "upload";

const AVATAR_UPLOAD_MAX_BYTES = 8 * 1024 * 1024;
const AVATAR_CROP_SIZE = 256;
const AVATAR_MIN_ZOOM = 1;
const AVATAR_MAX_ZOOM = 3;
const AVATAR_ZOOM_STEP = 0.15;

type CropOffset = { x: number; y: number };
type CropStageSize = { width: number; height: number };
type ImageSize = { width: number; height: number };

export function defaultAgentAvatar(): string {
  return AGENT_AVATAR_OPTIONS[0]?.value || "";
}

export function normalizeAgentAvatarPath(value: unknown): string {
  return normalizeAvatarPath(value);
}

export function AgentAvatarImage({ avatar, alt = "" }: { avatar?: string | null; alt?: string }) {
  const src = normalizeAgentAvatarPath(avatar);
  if (!src) {
    return <AgentIcon />;
  }
  return <img className="agent-avatar-image" src={src} alt={alt} draggable={false} />;
}

export function AgentAvatarContent({
  avatar,
  fallback,
  alt = "",
}: {
  avatar?: string | null;
  fallback?: ReactNode;
  alt?: string;
}) {
  const src = normalizeAgentAvatarPath(avatar);
  if (!src) {
    return <span className="agent-avatar-fallback">{fallback ?? avatar}</span>;
  }
  return <img className="agent-avatar-image" src={src} alt={alt} draggable={false} />;
}

export function AgentAvatarPicker({
  disabled = false,
  value,
  t,
  onChange,
  mode = "default",
}: {
  disabled?: boolean;
  value?: string | null;
  t: TranslateFn;
  onChange: (value: string) => void;
  mode?: "default" | "edit";
}) {
  const selected = normalizeAgentAvatarPath(value);
  const [open, setOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<AvatarEditorTab>("builtin");
  const [draftValue, setDraftValue] = useState(selected);
  const [uploadSource, setUploadSource] = useState("");
  const [uploadError, setUploadError] = useState("");
  const [uploadZoom, setUploadZoom] = useState(1);
  const [uploadOffset, setUploadOffset] = useState<CropOffset>({ x: 0, y: 0 });
  const [uploadImageSize, setUploadImageSize] = useState<ImageSize | null>(null);
  const pickerRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const [suppressTriggerHover, setSuppressTriggerHover] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const cropStageRef = useRef<HTMLDivElement | null>(null);
  const cropDragRef = useRef<{ pointerId: number; startX: number; startY: number; startOffset: CropOffset } | null>(
    null,
  );
  const selectedOption = AGENT_AVATAR_OPTIONS.find((option) => option.value === selected);
  const selectedLabel = selectedOption ? `${t(selectedOption.labelKey)} ${selectedOption.index}` : t("agentAvatar");
  const triggerLabel =
    mode === "edit"
      ? `${t("editAvatar")}${selectedOption ? `: ${selectedLabel}` : ""}`
      : selected
        ? `${t("agentAvatar")}: ${selectedLabel}`
        : t("agentAvatar");

  const closeEditor = useCallback(() => {
    setOpen(false);
    if (mode === "edit") {
      triggerRef.current?.blur();
      setSuppressTriggerHover(true);
    }
  }, [mode]);

  useEffect(() => {
    if (!open) {
      return;
    }
    function handlePointerDown(event: PointerEvent) {
      if (!pickerRef.current?.contains(event.target as Node)) {
        closeEditor();
      }
    }
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [closeEditor, open]);

  useEffect(() => {
    if (open) {
      setActiveTab("builtin");
      setDraftValue(selected);
      setUploadSource("");
      setUploadError("");
      setUploadZoom(1);
      setUploadOffset({ x: 0, y: 0 });
      setUploadImageSize(null);
    }
  }, [open, selected]);

  useEffect(() => {
    if (!uploadSource || !uploadImageSize) {
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      const stageSize = getCropStageSize(cropStageRef.current);
      const nextZoom = getBoundedCropZoom(uploadZoom, uploadImageSize, stageSize);
      setUploadZoom(nextZoom);
      setUploadOffset((current) => clampCropOffset(current, nextZoom, uploadImageSize, stageSize));
    });
    return () => window.cancelAnimationFrame(frame);
  }, [uploadSource, uploadImageSize, uploadZoom]);

  useEffect(() => {
    if (!open || mode !== "edit") {
      return;
    }
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        closeEditor();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [closeEditor, open, mode]);

  function handleTriggerPointerLeave() {
    setSuppressTriggerHover(false);
  }

  async function confirmDraft() {
    if (activeTab === "upload" && uploadSource) {
      try {
        onChange(
          await cropImageToAvatar(uploadSource, uploadZoom, uploadOffset, getCropStageSize(cropStageRef.current)),
        );
        closeEditor();
      } catch {
        setUploadError(t("avatarUploadReadFailed"));
      }
      return;
    }
    if (activeTab === "builtin" && draftValue) {
      onChange(draftValue);
    }
    closeEditor();
  }

  function updateZoom(direction: "in" | "out") {
    const stageSize = getCropStageSize(cropStageRef.current);
    const next = Number((uploadZoom + (direction === "in" ? AVATAR_ZOOM_STEP : -AVATAR_ZOOM_STEP)).toFixed(2));
    const bounded = uploadImageSize
      ? getBoundedCropZoom(next, uploadImageSize, stageSize)
      : Math.min(AVATAR_MAX_ZOOM, Math.max(AVATAR_MIN_ZOOM, next));
    setUploadZoom(bounded);
    if (uploadImageSize) {
      setUploadOffset((offset) => clampCropOffset(offset, bounded, uploadImageSize, stageSize));
    }
  }

  async function acceptUploadFile(file: File | undefined) {
    setUploadError("");
    if (!file) {
      return;
    }
    if (!file.type.startsWith("image/")) {
      setUploadError(t("avatarUploadInvalidType"));
      return;
    }
    if (file.size > AVATAR_UPLOAD_MAX_BYTES) {
      setUploadError(t("avatarUploadTooLarge"));
      return;
    }
    try {
      const dataUrl = await readFileAsDataURL(file);
      const imageSize = await ensureImageCanLoad(dataUrl);
      setUploadSource(dataUrl);
      setUploadImageSize(imageSize);
      setUploadZoom(1);
      setUploadOffset({ x: 0, y: 0 });
    } catch {
      setUploadError(t("avatarUploadReadFailed"));
    }
  }

  function handleCropPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    cropDragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      startOffset: uploadOffset,
    };
  }

  function handleCropPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const drag = cropDragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) {
      return;
    }
    const nextOffset = {
      x: drag.startOffset.x + event.clientX - drag.startX,
      y: drag.startOffset.y + event.clientY - drag.startY,
    };
    const stageSize = getCropStageSize(cropStageRef.current);
    setUploadOffset(uploadImageSize ? clampCropOffset(nextOffset, uploadZoom, uploadImageSize, stageSize) : nextOffset);
  }

  function stopCropDrag(event: ReactPointerEvent<HTMLDivElement>) {
    if (cropDragRef.current?.pointerId === event.pointerId) {
      cropDragRef.current = null;
    }
  }

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    void acceptUploadFile(event.currentTarget.files?.[0]);
    event.currentTarget.value = "";
  }

  function handleDrop(event: DragEvent<HTMLButtonElement>) {
    event.preventDefault();
    void acceptUploadFile(event.dataTransfer.files?.[0]);
  }

  function preventDefaultDrag(event: DragEvent<HTMLButtonElement>) {
    event.preventDefault();
  }

  function renderAvatarOption(
    option: (typeof AGENT_AVATAR_OPTIONS)[number],
    selection: string,
    onSelect: () => void,
    optionDisabled = false,
  ) {
    const checked = option.value === selection;
    const label = `${t(option.labelKey)} ${option.index}`;
    return (
      <button
        aria-checked={checked}
        aria-label={label}
        className={`agent-avatar-option ${checked ? "selected" : ""}`}
        key={option.value}
        role="radio"
        title={label}
        type="button"
        disabled={optionDisabled}
        onClick={onSelect}
      >
        <img src={option.value} alt="" draggable={false} />
      </button>
    );
  }

  return (
    <div
      className={`agent-avatar-picker ${mode === "edit" ? "edit-mode" : ""} ${selected ? "has-avatar" : "empty-avatar"} ${open ? "open" : ""} ${suppressTriggerHover ? "suppress-trigger-hover" : ""}`}
      aria-disabled={disabled}
      ref={pickerRef}
    >
      <button
        ref={triggerRef}
        aria-expanded={open}
        aria-haspopup="dialog"
        aria-label={triggerLabel}
        className="agent-avatar-trigger"
        title={mode === "edit" ? undefined : selected ? selectedLabel : t("agentAvatar")}
        type="button"
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        onPointerLeave={mode === "edit" ? handleTriggerPointerLeave : undefined}
      >
        {selected ? (
          <img className="agent-avatar-trigger-image" src={selected} alt="" draggable={false} />
        ) : (
          <ImagePlus aria-hidden="true" size={16} strokeWidth={1.8} />
        )}
        {mode === "edit" ? (
          <>
            {selected ? (
              <span className="agent-avatar-edit-overlay" aria-hidden="true">
                <Edit3 size={20} strokeWidth={1.8} />
              </span>
            ) : null}
            <span className="agent-avatar-edit-tooltip" role="tooltip">
              {t("editAvatar")}
            </span>
          </>
        ) : null}
      </button>
      {open && mode === "edit" && !disabled
        ? createPortal(
            <div className="agent-avatar-editor-backdrop" role="presentation" onPointerDown={closeEditor}>
              <div
                className="agent-avatar-editor-modal"
                role="dialog"
                aria-modal="true"
                aria-labelledby="agent-avatar-editor-title"
                onPointerDown={(event) => event.stopPropagation()}
              >
                <div className="agent-avatar-editor-header">
                  <div className="agent-avatar-editor-heading">
                    <h2 id="agent-avatar-editor-title">{t("editAvatar")}</h2>
                    <p>{t("editAvatarSubtitle")}</p>
                  </div>
                  <button
                    className="agent-avatar-editor-close"
                    type="button"
                    aria-label={t("close")}
                    onClick={closeEditor}
                  >
                    <X aria-hidden="true" size={24} strokeWidth={1.8} />
                  </button>
                </div>
                <div className="agent-avatar-editor-body">
                  <div className="agent-avatar-editor-tabs" role="tablist" aria-label={t("editAvatar")}>
                    {[
                      ["builtin", t("avatarBuiltinTab")],
                      ["upload", t("avatarUploadTab")],
                    ].map(([tab, label]) => (
                      <button
                        aria-selected={activeTab === tab}
                        className={`agent-avatar-editor-tab ${activeTab === tab ? "active" : ""}`}
                        key={tab}
                        role="tab"
                        type="button"
                        onClick={() => setActiveTab(tab as AvatarEditorTab)}
                      >
                        {label}
                      </button>
                    ))}
                  </div>
                  {activeTab === "builtin" ? (
                    <div className="agent-avatar-editor-grid" role="radiogroup" aria-label={t("avatarBuiltinTab")}>
                      {AGENT_AVATAR_OPTIONS.map((option) =>
                        renderAvatarOption(option, draftValue || selected, () => setDraftValue(option.value)),
                      )}
                    </div>
                  ) : (
                    <div className="agent-avatar-upload-pane" role="tabpanel" aria-label={t("avatarUploadTab")}>
                      {uploadSource ? (
                        <div className="agent-avatar-cropper">
                          <div
                            className="agent-avatar-cropper-stage"
                            ref={cropStageRef}
                            onPointerDown={handleCropPointerDown}
                            onPointerMove={handleCropPointerMove}
                            onPointerUp={stopCropDrag}
                            onPointerCancel={stopCropDrag}
                          >
                            <img
                              className="agent-avatar-cropper-image"
                              src={uploadSource}
                              alt={t("avatarUploadPreviewAlt")}
                              draggable={false}
                              style={{
                                transform: `translate(calc(-50% + ${uploadOffset.x}px), calc(-50% + ${uploadOffset.y}px)) scale(${uploadZoom})`,
                              }}
                            />
                            <div className="agent-avatar-cropper-frame" aria-hidden="true">
                              <span />
                              <span />
                              <span />
                              <span />
                            </div>
                            <div
                              className="agent-avatar-zoom-controls"
                              aria-label={t("avatarZoomControls")}
                              onPointerDown={(event) => event.stopPropagation()}
                            >
                              <button type="button" aria-label={t("avatarZoomOut")} onClick={() => updateZoom("out")}>
                                <ZoomOut aria-hidden="true" size={20} strokeWidth={1.8} />
                              </button>
                              <button type="button" aria-label={t("avatarZoomIn")} onClick={() => updateZoom("in")}>
                                <ZoomIn aria-hidden="true" size={20} strokeWidth={1.8} />
                              </button>
                            </div>
                          </div>
                        </div>
                      ) : (
                        <button
                          className={`agent-avatar-upload-dropzone ${uploadError ? "error" : ""}`}
                          type="button"
                          onClick={() => fileInputRef.current?.click()}
                          onDragEnter={preventDefaultDrag}
                          onDragOver={preventDefaultDrag}
                          onDrop={handleDrop}
                        >
                          <span className="agent-avatar-upload-icon" aria-hidden="true">
                            <UploadCloud size={20} strokeWidth={1.8} />
                          </span>
                          <span className="agent-avatar-upload-copy">
                            <span>
                              <strong>{t("avatarUploadClick")}</strong>
                              {t("avatarUploadDrop")}
                            </span>
                            <small>{t("avatarUploadHelp")}</small>
                          </span>
                          {uploadError ? <span className="agent-avatar-upload-error">{uploadError}</span> : null}
                        </button>
                      )}
                      <input
                        ref={fileInputRef}
                        className="agent-avatar-upload-input"
                        type="file"
                        accept="image/svg+xml,image/png,image/jpeg,image/gif"
                        onChange={handleFileChange}
                      />
                    </div>
                  )}
                </div>
                <div className="agent-avatar-editor-actions">
                  <button className="agent-avatar-editor-button secondary" type="button" onClick={closeEditor}>
                    {t("cancel")}
                  </button>
                  <button
                    className="agent-avatar-editor-button primary"
                    type="button"
                    disabled={activeTab === "upload" && !uploadSource}
                    onClick={() => void confirmDraft()}
                  >
                    {t("confirm")}
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
      {open && mode !== "edit" && !disabled ? (
        <div className="agent-avatar-picker-panel" role="radiogroup" aria-label={t("agentAvatar")}>
          {AVATAR_GROUPS.map((group) => (
            <div className="agent-avatar-picker-group" key={group.key}>
              <div className="agent-avatar-picker-label">{t(group.labelKey)}</div>
              <div className="agent-avatar-picker-options">
                {AGENT_AVATAR_OPTIONS.filter((option) => option.group === group.key).map((option) =>
                  renderAvatarOption(
                    option,
                    selected,
                    () => {
                      onChange(option.value);
                      setOpen(false);
                    },
                    disabled,
                  ),
                )}
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    reader.onload = () => resolve(String(reader.result || ""));
    reader.readAsDataURL(file);
  });
}

function ensureImageCanLoad(src: string): Promise<ImageSize> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight });
    image.onerror = reject;
    image.src = src;
  });
}

function getCropStageSize(stage: HTMLDivElement | null): CropStageSize {
  const rect = stage?.getBoundingClientRect();
  if (!rect?.width || !rect?.height) {
    return { width: 432, height: 276 };
  }
  return { width: rect.width, height: rect.height };
}

function getCropGeometry(stageSize: CropStageSize, imageSize: ImageSize, zoom: number) {
  const cropCenterX = stageSize.width / 2;
  const cropCenterY = stageSize.height / 2;
  const baseScale = Math.min(stageSize.width / imageSize.width, stageSize.height / imageSize.height);
  const scale = baseScale * zoom;
  const width = imageSize.width * scale;
  const height = imageSize.height * scale;
  return {
    cropCenterX,
    cropCenterY,
    cropLeft: cropCenterX - AVATAR_CROP_SIZE / 2,
    cropRight: cropCenterX + AVATAR_CROP_SIZE / 2,
    cropTop: cropCenterY - AVATAR_CROP_SIZE / 2,
    cropBottom: cropCenterY + AVATAR_CROP_SIZE / 2,
    height,
    width,
  };
}

function getMinimumCropZoom(imageSize: ImageSize, stageSize: CropStageSize): number {
  const baseScale = Math.min(stageSize.width / imageSize.width, stageSize.height / imageSize.height);
  if (!Number.isFinite(baseScale) || baseScale <= 0) {
    return AVATAR_MIN_ZOOM;
  }
  const minZoomForWidth = AVATAR_CROP_SIZE / (imageSize.width * baseScale);
  const minZoomForHeight = AVATAR_CROP_SIZE / (imageSize.height * baseScale);
  return Math.min(AVATAR_MAX_ZOOM, Math.max(AVATAR_MIN_ZOOM, minZoomForWidth, minZoomForHeight));
}

function getBoundedCropZoom(zoom: number, imageSize: ImageSize, stageSize: CropStageSize): number {
  const minimumZoom = getMinimumCropZoom(imageSize, stageSize);
  const maximumZoom = Math.max(AVATAR_MAX_ZOOM, minimumZoom);
  return Math.min(maximumZoom, Math.max(minimumZoom, zoom));
}

function clampAxisOffset(
  current: number,
  imageLength: number,
  cropStart: number,
  cropEnd: number,
  cropCenter: number,
): number {
  const minOffset = cropEnd - cropCenter - imageLength / 2;
  const maxOffset = cropStart - cropCenter + imageLength / 2;
  if (minOffset > maxOffset) {
    return 0;
  }
  return Math.min(maxOffset, Math.max(minOffset, current));
}

function clampCropOffset(offset: CropOffset, zoom: number, imageSize: ImageSize, stageSize: CropStageSize): CropOffset {
  const geometry = getCropGeometry(stageSize, imageSize, zoom);
  return {
    x: clampAxisOffset(offset.x, geometry.width, geometry.cropLeft, geometry.cropRight, geometry.cropCenterX),
    y: clampAxisOffset(offset.y, geometry.height, geometry.cropTop, geometry.cropBottom, geometry.cropCenterY),
  };
}

function cropImageToAvatar(src: string, zoom: number, offset: CropOffset, stageSize: CropStageSize): Promise<string> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = AVATAR_CROP_SIZE;
      canvas.height = AVATAR_CROP_SIZE;
      const context = canvas.getContext("2d");
      if (!context) {
        reject(new Error("Canvas is unavailable."));
        return;
      }
      const imageSize = { width: image.naturalWidth, height: image.naturalHeight };
      const boundedZoom = getBoundedCropZoom(zoom, imageSize, stageSize);
      const boundedOffset = clampCropOffset(offset, boundedZoom, imageSize, stageSize);
      const cropGeometry = getCropGeometry(stageSize, imageSize, boundedZoom);
      const cropCenterX = cropGeometry.cropCenterX;
      const cropCenterY = cropGeometry.cropCenterY;
      const imageCenterX = cropCenterX + boundedOffset.x;
      const imageCenterY = cropCenterY + boundedOffset.y;
      const width = cropGeometry.width;
      const height = cropGeometry.height;
      const imageLeftInStage = imageCenterX - width / 2;
      const imageTopInStage = imageCenterY - height / 2;
      const cropLeftInStage = cropGeometry.cropLeft;
      const cropTopInStage = cropGeometry.cropTop;

      context.drawImage(image, imageLeftInStage - cropLeftInStage, imageTopInStage - cropTopInStage, width, height);
      resolve(canvas.toDataURL("image/png"));
    };
    image.onerror = reject;
    image.src = src;
  });
}
