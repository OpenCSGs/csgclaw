import { useNativeBrowser } from "./useNativeBrowser";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Copy, ExternalLink, RefreshCw } from "lucide-react";
import { Button, EmptyState, TextInput, Tooltip } from "@/components/ui";
import { normalizeBrowserURL } from "@/models/fileReferences";
import type { TranslateFn } from "@/models/conversations";

export function BrowserTab({
  url,
  active,
  onNavigate,
  onPopup,
  t,
}: {
  url: string;
  active: boolean;
  onNavigate: (url: string) => void;
  onPopup: (url: string) => void;
  t: TranslateFn;
}) {
  const [address, setAddress] = useState(url);
  const [reload, setReload] = useState(0);
  const [invalid, setInvalid] = useState(false);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  const [history, setHistory] = useState(() => ({ urls: url ? [url] : [], index: url ? 0 : -1 }));
  const previousURL = useRef(url);
  useEffect(() => {
    const previous = previousURL.current;
    previousURL.current = url;
    setAddress((current) => (current === previous ? url : current));
  }, [url]);
  const native = useNativeBrowser({ url, active, onNavigate, onPopup });
  useEffect(() => {
    if (!native.available && url) {
      setHistory((current) =>
        current.urls[current.index] === url
          ? current
          : { urls: [...current.urls.slice(0, current.index + 1), url], index: current.index + 1 },
      );
    }
  }, [native.available, url]);
  const externalURL = normalizeBrowserURL(address) || url;
  return (
    <section className="chat-browser-tab" aria-label={t("chatBrowser")}>
      <form
        className="chat-browser-toolbar"
        onSubmit={(event) => {
          event.preventDefault();
          const next = normalizeBrowserURL(address);
          setInvalid(!next);
          if (next) {
            setCopied(false);
            setCopyError(false);
            if (!native.available && next !== url) {
              setHistory((current) => ({
                urls: [...current.urls.slice(0, current.index + 1), next],
                index: current.index + 1,
              }));
            }
            onNavigate(next);
            setAddress(next);
            if (native.available && next === url) void native.command("reload");
            else setReload((value) => value + 1);
          }
        }}
      >
        <Tooltip content={t("chatBrowserBack")} contentProps={{ side: "bottom" }}>
          <span className="inline-flex">
            <Button
              type="button"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatBrowserBack")}
              disabled={native.available ? !native.state?.canGoBack : history.index <= 0}
              onClick={() => {
                if (native.available) void native.command("back");
                else {
                  const next = history.index - 1;
                  setHistory((current) => ({ ...current, index: next }));
                  onNavigate(history.urls[next]);
                }
              }}
            >
              <ArrowLeft size={16} />
            </Button>
          </span>
        </Tooltip>
        <Tooltip content={t("chatBrowserForward")} contentProps={{ side: "bottom" }}>
          <span className="inline-flex">
            <Button
              type="button"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatBrowserForward")}
              disabled={native.available ? !native.state?.canGoForward : history.index >= history.urls.length - 1}
              onClick={() => {
                if (native.available) void native.command("forward");
                else {
                  const next = history.index + 1;
                  setHistory((current) => ({ ...current, index: next }));
                  onNavigate(history.urls[next]);
                }
              }}
            >
              <ArrowRight size={16} />
            </Button>
          </span>
        </Tooltip>
        <Tooltip content={t("chatRefresh")} contentProps={{ side: "bottom" }}>
          <span className="inline-flex">
            <Button
              type="button"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatRefresh")}
              disabled={!url}
              onClick={() => (native.available ? void native.command("reload") : setReload((value) => value + 1))}
            >
              <RefreshCw size={16} />
            </Button>
          </span>
        </Tooltip>
        <TextInput
          className="chat-browser-address"
          size="sm"
          aria-label={t("chatBrowserAddress")}
          placeholder={t("chatBrowserAddressPlaceholder")}
          autoFocus={!url}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={address}
          onChange={(event) => {
            setAddress(event.target.value);
            setInvalid(false);
          }}
          aria-invalid={invalid}
        />
        <Tooltip content={t(copied ? "chatCopied" : "chatCopyURL")} contentProps={{ side: "bottom" }}>
          <span className="inline-flex">
            <Button
              type="button"
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatCopyURL")}
              disabled={!url}
              onClick={() => {
                void writeTextToClipboard(url).then((ok) => {
                  setCopied(ok);
                  setCopyError(!ok);
                });
              }}
            >
              <Copy size={16} />
            </Button>
          </span>
        </Tooltip>
        {externalURL ? (
          <Tooltip content={t("chatBrowserExternal")} contentProps={{ side: "bottom" }}>
            <a
              className="chat-browser-external"
              href={externalURL}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={t("chatBrowserExternal")}
            >
              <ExternalLink size={16} />
            </a>
          </Tooltip>
        ) : null}
      </form>
      {copyError ? (
        <div className="chat-browser-hint" role="alert">
          {t("copyTextFailed")}
        </div>
      ) : null}
      {invalid ? (
        <div className="chat-browser-hint" role="alert">
          {t("chatBrowserInvalid")}
        </div>
      ) : null}
      {native.available && url ? (
        <div className="chat-browser-native" ref={native.hostRef}>
          {native.failed ? (
            <div className="chat-side-status" role="alert">
              <span>{t("chatBrowserLoadFailed")}</span>
              <Button size="sm" variant="secondaryGray" onClick={() => void native.command("reload")}>
                {t("chatBrowserRetry")}
              </Button>
            </div>
          ) : null}
        </div>
      ) : url ? (
        <>
          <div className="chat-browser-hint chat-browser-embed-hint">
            <span>{t("chatBrowserEmbedHint")}</span>
            <a href={externalURL} target="_blank" rel="noopener noreferrer">
              <ExternalLink size={14} />
              {t("chatBrowserExternal")}
            </a>
          </div>
          <iframe
            key={`${url}:${reload}`}
            src={url}
            title={t("chatBrowser")}
            sandbox="allow-scripts allow-forms"
            referrerPolicy="no-referrer"
          />
        </>
      ) : (
        <EmptyState
          className="chat-browser-empty"
          title={t("chatBrowserEmptyTitle")}
          description={t("chatBrowserEmptyDescription")}
        />
      )}
    </section>
  );
}
