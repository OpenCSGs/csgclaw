import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Button } from "@/components/ui";

export type ClipboardCopyButtonProps = {
  className?: string;
  disabled?: boolean;
  icon?: ReactNode;
  iconOnly?: boolean;
  label: string;
  copiedIcon?: ReactNode;
  copiedLabel?: string;
  text?: string;
};

async function copyTextToClipboard(text: string) {
  if (!text) {
    return;
  }
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);
  }
}

export function ClipboardCopyButton({
  className,
  copiedIcon,
  copiedLabel,
  disabled,
  icon,
  iconOnly = false,
  label,
  text,
}: ClipboardCopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);
  const value = String(text ?? "");
  const busy = Boolean(disabled) || !value.trim();

  useEffect(
    () => () => {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
      }
    },
    [],
  );

  async function handleClick() {
    if (busy) {
      return;
    }
    await copyTextToClipboard(value);
    setCopied(true);
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
    }
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null;
      setCopied(false);
    }, 2000);
  }

  return (
    <Button
      aria-label={copied ? copiedLabel || label : label}
      className={className || "notifier-copy-button"}
      disabled={busy}
      iconOnly={iconOnly}
      onClick={handleClick}
      style={copied ? { background: "#16a34a", borderColor: "transparent", color: "#fff" } : undefined}
      title={copied ? copiedLabel || label : label}
    >
      {copied ? copiedIcon || "OK" : icon || label}
    </Button>
  );
}
