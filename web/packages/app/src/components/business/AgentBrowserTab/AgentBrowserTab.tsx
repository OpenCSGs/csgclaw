import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ArrowLeft, ArrowRight, Camera, ExternalLink, RefreshCw } from "lucide-react";
import { Button, TextInput, Tooltip } from "@/components/ui";
import {
  browserCommand,
  takeBrowserControl,
  releaseBrowserControl,
  BrowserControlExpired,
  browserScreenshotURL,
  fetchBrowserFrame,
  type BrowserCommand,
  type BrowserSession,
} from "@/api/browser";
import { normalizeBrowserURL } from "@/models/fileReferences";
import type { TranslateFn } from "@/models/conversations";
import styles from "./AgentBrowserTab.module.css";

const keys = new Set([
  "Enter",
  "Space",
  "Tab",
  "Escape",
  "Backspace",
  "Delete",
  "ArrowDown",
  "ArrowUp",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
  "PageDown",
  "PageUp",
]);

export function AgentBrowserTab({
  roomID,
  session,
  active,
  t,
}: {
  roomID: string;
  session: BrowserSession;
  active: boolean;
  t: TranslateFn;
}) {
  const [address, setAddress] = useState(session.url);
  const [frame, setFrame] = useState("");
  const [controlToken, setControlToken] = useState("");
  const [frameControl, setFrameControl] = useState("");
  const [frameRevision, setFrameRevision] = useState("");
  const [inputReady, setInputReady] = useState(false);
  const displayedRevision = useRef("");
  const [changing, setChanging] = useState(false);
  const changingRef = useRef(false);
  const controlled = Boolean(controlToken && frameControl === controlToken && frameRevision && inputReady);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [retry, setRetry] = useState(0);
  const image = useRef<HTMLImageElement>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const queue = useRef(Promise.resolve());
  const mounted = useRef(true);
  const controlEpoch = useRef(0);
  useLayoutEffect(() => {
    const epoch = ++controlEpoch.current;
    return () => {
      controlEpoch.current = epoch + 1;
    };
  }, [roomID, session.id, active]);
  const revision = useRef(0);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    setAddress(session.url);
  }, [session.url]);
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    let objectURL = "";
    async function poll() {
      try {
        const captured = await fetchBrowserFrame(roomID, session.id, controller.signal, controlToken);
        const blob = captured.blob;
        if (controller.signal.aborted) return;
        const next = URL.createObjectURL(blob);
        const previous = objectURL;
        objectURL = next;
        if (controlToken && captured.revision !== displayedRevision.current) setInputReady(false);
        setFrame(next);
        setFrameControl(controlToken);
        setFrameRevision(captured.revision);
        if (previous) URL.revokeObjectURL(previous);
      } catch (cause) {
        if (!controller.signal.aborted) {
          setError(true);
          if (cause instanceof BrowserControlExpired) {
            setControlToken("");
            setInputReady(false);
          }
        }
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 800);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [roomID, session.id, active, retry, controlToken]);

  useEffect(() => {
    if (!controlToken || !active) return;
    return () => {
      void releaseBrowserControl(roomID, session.id, controlToken).catch(() => {});
    };
  }, [roomID, session.id, active, controlToken]);
  useEffect(() => {
    if (!active) {
      setControlToken("");
      setInputReady(false);
      setFrame("");
    }
  }, [active]);
  async function toggleControl() {
    const epoch = controlEpoch.current;
    changingRef.current = true;
    setChanging(true);
    setError(false);
    try {
      await queue.current;
      if (!mounted.current || !active || epoch !== controlEpoch.current) return;
      if (controlToken) {
        await releaseBrowserControl(roomID, session.id, controlToken);
        if (mounted.current) setControlToken("");
      } else {
        const control = await takeBrowserControl(roomID, session.id);
        if (!mounted.current || !active || epoch !== controlEpoch.current) {
          await releaseBrowserControl(roomID, session.id, control.token);
          return;
        }
        setFrame("");
        setFrameControl("");
        setFrameRevision("");
        setInputReady(false);
        setControlToken(control.token);
      }
    } catch {
      if (mounted.current) setError(true);
    } finally {
      changingRef.current = false;
      if (mounted.current) setChanging(false);
    }
  }
  function command(value: BrowserCommand) {
    if (changingRef.current || (value.action !== "resize" && !controlled)) return Promise.resolve(false);
    if (value.action === "resize" && session.user_controlled && !controlToken) return Promise.resolve(false);
    const current = ++revision.current;
    setBusy(true);
    setError(false);
    const pending = queue.current.then(async () => {
      if (!mounted.current) return false;
      try {
        const result = await browserCommand(roomID, session.id, value, controlToken, frameRevision);
        if (mounted.current && value.action === "navigate") setAddress(result.url);
        return true;
      } catch {
        if (mounted.current) setError(true);
        return false;
      } finally {
        if (mounted.current && current === revision.current) setBusy(false);
      }
    });
    queue.current = pending.then(() => {});
    return pending;
  }
  const send = useRef(command);
  send.current = command;
  useEffect(() => {
    const host = viewport.current;
    if (!active || !host || typeof ResizeObserver === "undefined") return;
    let timer: ReturnType<typeof setTimeout>;
    let previous = "";
    const observer = new ResizeObserver(() => {
      const width = Math.max(320, Math.min(1920, Math.round(host.clientWidth)));
      const height = Math.max(200, Math.min(1440, Math.round(host.clientHeight)));
      const size = `${width}:${height}`;
      if (size === previous) return;
      previous = size;
      clearTimeout(timer);
      timer = setTimeout(() => send.current({ action: "resize", width, height }), 180);
    });
    observer.observe(host);
    return () => {
      observer.disconnect();
      clearTimeout(timer);
    };
  }, [active, session.id]);
  useEffect(() => {
    const host = viewport.current;
    if (!host || !active || !controlled) return;
    let dx = 0,
      dy = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const wheel = (event: WheelEvent) => {
      event.preventDefault();
      const scale = event.deltaMode === 1 ? 20 : event.deltaMode === 2 ? host.clientHeight : 1;
      dx += event.deltaX * scale;
      dy += event.deltaY * scale;
      timer ??= setTimeout(() => {
        void send.current({
          action: "scroll",
          x: Math.max(-10000, Math.min(10000, dx)),
          y: Math.max(-10000, Math.min(10000, dy)),
        });
        dx = 0;
        dy = 0;
        timer = undefined;
      }, 100);
    };
    host.addEventListener("wheel", wheel, { passive: false });
    return () => {
      host.removeEventListener("wheel", wheel);
      clearTimeout(timer);
    };
  }, [active, controlled]);
  const toolbarButton = (label: string, icon: ReactNode, action: BrowserCommand["action"], disabled = false) => (
    <Tooltip content={label}>
      <span className="inline-flex">
        <Button
          iconOnly
          size="sm"
          variant="tertiaryGray"
          aria-label={label}
          disabled={disabled || busy || !controlled}
          onClick={() => command({ action })}
        >
          {icon}
        </Button>
      </span>
    </Tooltip>
  );
  return (
    <section className={styles.root} aria-label={t("chatAgentBrowser")} aria-busy={busy}>
      <form
        className={styles.toolbar}
        onSubmit={(event) => {
          event.preventDefault();
          if (busy || !controlled) return;
          const url = normalizeBrowserURL(address);
          setInvalid(!url);
          if (!url) {
            return;
          }
          command({ action: "navigate", url });
        }}
      >
        {toolbarButton(t("chatBrowserBack"), <ArrowLeft size={16} />, "back", !session.can_go_back)}
        {toolbarButton(t("chatBrowserForward"), <ArrowRight size={16} />, "forward", !session.can_go_forward)}
        {toolbarButton(t("chatRefresh"), <RefreshCw size={16} />, "reload")}
        <TextInput
          className={styles.address}
          size="sm"
          aria-label={t("chatBrowserAddress")}
          value={address}
          disabled={!controlled || busy}
          aria-invalid={invalid}
          onChange={(event) => {
            setAddress(event.target.value);
            setInvalid(false);
          }}
          autoCapitalize="none"
          spellCheck={false}
        />
        <Tooltip content={t("chatBrowserScreenshot")}>
          <a
            className={styles.link}
            href={`${browserScreenshotURL(roomID, session.id)}?full_page=true`}
            download="browser.png"
            aria-label={t("chatBrowserScreenshot")}
          >
            <Camera size={16} />
          </a>
        </Tooltip>
        <Tooltip content={t("chatBrowserSystem")}>
          <a
            className={styles.link}
            href={session.url}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={t("chatBrowserSystem")}
          >
            <ExternalLink size={16} />
          </a>
        </Tooltip>
      </form>
      <div className={styles.control}>
        <p className={styles.hint}>
          {t(
            controlToken
              ? "chatAgentBrowserControlled"
              : session.user_controlled
                ? "chatAgentBrowserOtherControl"
                : "chatAgentBrowserHint",
          )}
        </p>
        <Button
          size="sm"
          variant="secondaryGray"
          disabled={busy || changing || (!controlToken && Boolean(session.user_controlled))}
          onClick={() => void toggleControl()}
        >
          {t(
            changing
              ? "chatAgentBrowserControlPending"
              : controlToken
                ? "chatAgentBrowserReleaseControl"
                : "chatAgentBrowserTakeControl",
          )}
        </Button>
      </div>
      {invalid ? (
        <div role="alert" className={styles.status}>
          {t("chatBrowserInvalid")}
        </div>
      ) : null}
      {error ? (
        <div className={styles.status} role="alert">
          <span>{t("chatAgentBrowserFailed")}</span>
          <Button
            size="sm"
            variant="secondaryGray"
            onClick={() => {
              setError(false);
              setRetry((value) => value + 1);
            }}
          >
            {t("chatBrowserRetry")}
          </Button>
        </div>
      ) : null}
      <div className={styles.viewport} ref={viewport}>
        {frame ? (
          <img
            ref={image}
            className={styles.frame}
            src={frame}
            alt={t("chatAgentBrowserPage")}
            tabIndex={active && controlled ? 0 : -1}
            aria-disabled={!controlled}
            onLoad={() => {
              if (controlToken && frameControl === controlToken) {
                displayedRevision.current = frameRevision;
                setInputReady(true);
              }
            }}
            draggable={false}
            onClick={(event) => {
              if (!controlled) return;
              const bounds = event.currentTarget.getBoundingClientRect();
              event.currentTarget.focus();
              command({
                action: "pointer",
                x: Math.max(
                  0,
                  Math.min(
                    event.currentTarget.naturalWidth - 1,
                    ((event.clientX - bounds.left) * event.currentTarget.naturalWidth) / bounds.width,
                  ),
                ),
                y: Math.max(
                  0,
                  Math.min(
                    event.currentTarget.naturalHeight - 1,
                    ((event.clientY - bounds.top) * event.currentTarget.naturalHeight) / bounds.height,
                  ),
                ),
              });
            }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || event.ctrlKey || event.metaKey || event.altKey) return;
              if (event.key === "Tab" || event.key === "Escape") return;
              const key = event.key === " " ? "Space" : event.key;
              if (keys.has(key)) {
                event.preventDefault();
                command({ action: "key", key });
              } else if (event.key.length === 1) {
                event.preventDefault();
                command({ action: "type", text: event.key });
              }
            }}
            onPaste={(event) => {
              event.preventDefault();
              command({ action: "type", text: event.clipboardData.getData("text/plain") });
            }}
          />
        ) : (
          <div className={styles.status} role="status">
            {t("chatAgentBrowserLoading")}
          </div>
        )}
      </div>
      <form
        className={styles.input}
        onSubmit={(event) => {
          event.preventDefault();
          if (busy || !controlled) return;
          const form = event.currentTarget;
          const text = String(new FormData(form).get("text") ?? "");
          if (text) {
            void command({ action: "type", text }).then((success) => {
              if (success && mounted.current) form.reset();
            });
          }
        }}
      >
        <TextInput
          name="text"
          size="sm"
          aria-label={t("chatAgentBrowserText")}
          placeholder={t("chatAgentBrowserText")}
          disabled={!controlled || busy}
        />
        <Button type="submit" size="sm" variant="secondaryGray" disabled={busy || !controlled}>
          {t("chatAgentBrowserType")}
        </Button>
        <Button
          size="sm"
          variant="tertiaryGray"
          disabled={busy || !controlled}
          onClick={() => command({ action: "key", key: "Tab" })}
        >
          {t("chatAgentBrowserNext")}
        </Button>
        <Button
          size="sm"
          variant="tertiaryGray"
          disabled={busy || !controlled}
          onClick={() => command({ action: "key", key: "Enter" })}
        >
          {t("chatAgentBrowserEnter")}
        </Button>
      </form>
    </section>
  );
}
