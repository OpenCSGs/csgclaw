import { UploadCloud, DownloadCloud } from "lucide-react";
import { useState, type ReactNode } from "react";
import type { RelayPublicationController } from "@/hooks/workspace/useRelayPublication";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIErrorWithDiagnostics } from "@/shared/i18n";
import {
  Button,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogCloseButton,
  DropdownMenuItem,
} from "@/components/ui";

const SHOW_RELAY_PUBLISH_ACTION = false;

// Shared agent detail action; the focused controller owns its API lifecycle.
export function RelayPublication({
  controller,
  t,
  renderAction,
}: {
  controller: RelayPublicationController;
  t: TranslateFn;
  renderAction: (action?: ReactNode) => ReactNode;
}) {
  const { status, publish, unpublish } = controller;
  const [open, setOpen] = useState(false);
  const busy = publish.isPending || unpublish.isPending;
  const data = status.data;
  const canUnpublish = Boolean(data?.can_unpublish || data?.published);
  const cleanup = canUnpublish && (!data?.published || Boolean(data.removing));
  const actionLabel = cleanup ? "relayCancelPublication" : data?.published ? "relayUnpublish" : "relayPublish";
  const error = publish.error || unpublish.error || status.error;
  async function submit() {
    try {
      await publish.mutateAsync();
      setOpen(false);
    } catch {
      setOpen(true);
    }
  }
  return (
    <>
      {renderAction(
        canUnpublish || SHOW_RELAY_PUBLISH_ACTION ? (
          <DropdownMenuItem
            disabled={busy || status.isPending || status.isError}
            onSelect={() => {
              publish.reset();
              unpublish.reset();
              setOpen(true);
            }}
          >
            {canUnpublish ? (
              <DownloadCloud aria-hidden="true" size={15} strokeWidth={2} />
            ) : (
              <UploadCloud aria-hidden="true" size={15} strokeWidth={2} />
            )}
            <span>{busy ? t(unpublish.isPending ? "relayUnpublishing" : "agentPublishing") : t(actionLabel)}</span>
          </DropdownMenuItem>
        ) : undefined,
      )}
      {data?.published ? (
        <span role="status">
          {t(data.agent?.status === "online" ? (data.agent.busy ? "relayBusy" : "relayOnline") : "relayOffline")}
        </span>
      ) : null}
      {error ? <span role="alert">{localizeAPIErrorWithDiagnostics(error, t)}</span> : null}
      {data?.error ? <span role="alert">{t("relayStatusError")}</span> : null}
      <DialogRoot
        open={open}
        onOpenChange={(value) => {
          if (!busy) {
            setOpen(value);
          }
        }}
      >
        <DialogContent className="agent-publish-dialog">
          <DialogHeader>
            <div>
              <DialogTitle>{t(actionLabel)}</DialogTitle>
              <DialogDescription>
                {t(
                  cleanup
                    ? "relayCancelPublicationDescription"
                    : data?.published
                      ? "relayUnpublishDescription"
                      : "relayDescription",
                )}
              </DialogDescription>
            </div>
            <DialogCloseButton label={t("cancel")} disabled={busy} />
          </DialogHeader>
          <DialogBody>
            {data?.published ? <p>{data.agent?.id || t("relayOffline")}</p> : <p>{t("relayDescription")}</p>}
            {error ? <p role="alert">{localizeAPIErrorWithDiagnostics(error, t)}</p> : null}
          </DialogBody>
          <DialogFooter>
            <Button
              variant="secondaryGray"
              size="md"
              disabled={busy}
              onClick={() => {
                setOpen(false);
              }}
            >
              {t("cancel")}
            </Button>
            {SHOW_RELAY_PUBLISH_ACTION && data?.pending && !data.removing && !data.publishing ? (
              <Button variant="primary" size="md" disabled={busy} onClick={() => void submit()}>
                {t("retry")}
              </Button>
            ) : null}
            {canUnpublish ? (
              <Button
                variant="danger"
                size="md"
                disabled={busy}
                onClick={() => {
                  void unpublish
                    .mutateAsync()
                    .then(() => setOpen(false))
                    .catch(() => {});
                }}
              >
                {t(actionLabel)}
              </Button>
            ) : SHOW_RELAY_PUBLISH_ACTION ? (
              <Button variant="primary" size="md" disabled={busy} onClick={() => void submit()}>
                {t("relayPublish")}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </>
  );
}
