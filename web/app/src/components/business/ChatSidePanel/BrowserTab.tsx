import { useNativeBrowser } from "./useNativeBrowser";
import { writeTextToClipboard } from "@/shared/lib/clipboard";
import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Copy, ExternalLink, Globe, RefreshCw } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
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
  const previousURL = useRef(url);
  useEffect(() => {
    const previous = previousURL.current;
    previousURL.current = url;
    setAddress((current) => (current === previous ? url : current));
  }, [url]);
  const native = useNativeBrowser({ url, active, onNavigate, onPopup });
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
            onNavigate(next);
            setAddress(next);
            if (native.available && next === url) void native.command("reload");
            else setReload((value) => value + 1);
          }
        }}
      >
        {native.available ? (
          <>
            <Tooltip content={t("chatBrowserBack")}>
              <Button
                type="button"
                iconOnly
                size="sm"
                variant="tertiaryGray"
                aria-label={t("chatBrowserBack")}
                disabled={!native.state?.canGoBack}
                onClick={() => void native.command("back")}
              >
                <ArrowLeft size={16} />
              </Button>
            </Tooltip>
            <Tooltip content={t("chatBrowserForward")}>
              <Button
                type="button"
                iconOnly
                size="sm"
                variant="tertiaryGray"
                aria-label={t("chatBrowserForward")}
                disabled={!native.state?.canGoForward}
                onClick={() => void native.command("forward")}
              >
                <ArrowRight size={16} />
              </Button>
            </Tooltip>
          </>
        ) : null}
        <input
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
        <Button type="submit" size="sm" variant="secondaryGray">
          {t("chatBrowserGo")}
        </Button>
        <Tooltip content={t("chatRefresh")}>
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
        </Tooltip>
        <Tooltip content={t(copied ? "chatCopied" : "chatCopyURL")}>
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
        </Tooltip>
        {externalURL ? (
          <a
            className="chat-browser-external"
            href={externalURL}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={t("chatBrowserExternal")}
            title={t("chatBrowserExternal")}
          >
            <ExternalLink size={16} />
          </a>
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
        <div className="chat-side-status">
          <Globe size={36} />
          <strong>{t("chatBrowser")}</strong>
          <span>{t("chatBrowserEmpty")}</span>
        </div>
      )}
    </section>
  );
}
