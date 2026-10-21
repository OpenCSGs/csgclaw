import { useRef, useState } from "react";
import { Plus } from "lucide-react";
import { Button, Checkbox, DialogRoot, LinkButton, SegmentedControl, TextInput } from "@/components/ui";
import { SkillUploadDialog } from "@/components/business/SkillUploadDialog";
import { MCPServerCreateDialog } from "@/components/business/HubDetailPane";
import { useAgentResourcePicker, type AgentResourcePickerKind } from "@/hooks/workspace/useAgentResourcePicker";
import { knowledgeBaseMCPServer } from "@/models/knowledgeBases";
import { isRemoteMCPInstalled, mcpManagedKnowledgeBaseSource, mcpServerDisplayName } from "@/models/mcp";
import type { SkillSummary } from "@/models/skillhub";
import { ResourceFeaturedIcon } from "@/components/business/ResourceListCard";
import { SidebarMcpIcon, SidebarPuzzlePiece02Icon } from "@/components/ui/Icons";
import { SkillMarketPreview } from "./SkillMarketPreview";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";
import { errorMessage } from "@/api/client";
import {
  ResourcePicker,
  ResourcePickerList,
  ResourcePickerRow,
  ResourcePickerCopy,
  ResourcePickerActions,
} from "@/components/business/ResourcePicker";

export function AgentResourcePicker({
  agentID,
  kind,
  authenticated,
  error,
  onAdd,
  onClose,
  onLogin,
  portalContainer,
  t,
}: {
  agentID: string;
  kind: AgentResourcePickerKind;
  authenticated: boolean;
  error?: string;
  onAdd: (names: string[]) => Promise<boolean> | boolean;
  onClose: () => void;
  onLogin?: () => boolean;
  portalContainer?: HTMLElement | null;
  t: TranslateFn;
}) {
  const picker = useAgentResourcePicker({ agentID, kind, authenticated, onAdd, t });
  const [selected, setSelected] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [preview, setPreview] = useState<SkillSummary | null>(null);
  const previewTrigger = useRef<HTMLButtonElement | null>(null);
  const title = t(kind === "skill" ? "agentSkillAdd" : kind === "mcp" ? "agentMCPAdd" : "agentKnowledgeAdd");
  const busy = Boolean(picker.busy);
  const marketQuery = kind === "skill" ? picker.skillMarket : kind === "mcp" ? picker.mcpMarket : picker.knowledge;
  const loadError = picker.libraryError || ((picker.market || kind === "knowledge") && marketQuery.error);
  const failure = picker.error || error || (loadError ? localizeAPIError(loadError, t, errorMessage(loadError)) : "");
  const match = (name: string, description = "") =>
    `${name} ${description}`.toLowerCase().includes(picker.search.trim().toLowerCase());
  const library =
    kind === "skill"
      ? picker.skills
          .filter((skill) => !picker.installedNames.has(skill.name))
          .map((skill) => ({ name: skill.name, label: skill.name, description: skill.description }))
      : picker.globalServers
          .filter((server) => Boolean(mcpManagedKnowledgeBaseSource(server.config)) === (kind === "knowledge"))
          .map((server) => ({
            name: server.name,
            label: mcpServerDisplayName(server),
            description: server.description,
          }));
  const visibleLibrary = library.filter((item) => match(item.label, item.description));
  async function finish(operation: Promise<boolean>) {
    const added = await operation;
    if (added) onClose();
    return added;
  }
  function select(name: string, checked: boolean) {
    setSelected((current) => (checked ? [...current, name] : current.filter((item) => item !== name)));
  }
  const remoteRows =
    kind === "skill"
      ? (picker.skillMarket.data?.pages.flatMap((page) => page.items) ?? []).map((skill) => ({
          id: skill.remotePath || skill.name,
          open: (trigger: HTMLButtonElement) => {
            previewTrigger.current = trigger;
            setPreview(skill);
          },
          name: skill.name,
          description: skill.description,
          installed: picker.isSkillEnabled(skill),
          unavailable: false,
          add: () => picker.installSkill(skill),
        }))
      : kind === "mcp"
        ? (picker.mcpMarket.data?.pages.flatMap((page) => page.items) ?? []).map((server) => ({
            id: server.id,
            open: undefined,
            name: server.name,
            description: server.description,
            installed: picker.globalServers.some(
              (item) => picker.installedNames.has(item.name) && isRemoteMCPInstalled([item], server),
            ),
            unavailable: false,
            add: () => picker.installMCP(server),
          }))
        : picker.knowledgeItems
            .filter((item) => !knowledgeBaseMCPServer(item, picker.globalServers))
            .map((item) => ({
              id: item.id,
              open: undefined,
              name: item.name,
              description: item.unavailableReason || item.description,
              installed: false,
              unavailable: item.availability !== "available",
              add: () => picker.connectKnowledge(item),
            }));
  const marketLabel = t(kind === "skill" ? "profileSkillMarket" : "profileMCPMarket");
  const searchLabel = t(
    kind === "skill"
      ? picker.market
        ? "iaSkillsMarketSearchPlaceholder"
        : "resourcesSkillSearchPlaceholder"
      : picker.market
        ? "iaMCPMarketSearchPlaceholder"
        : "iaMCPInstalledSearchPlaceholder",
  );
  return (
    <>
      <DialogRoot
        open
        onOpenChange={(open) => {
          if (!open && !busy) onClose();
        }}
      >
        <ResourcePicker
          title={title}
          description={t(
            kind === "skill"
              ? "profileSkillPickerHint"
              : kind === "mcp"
                ? "profileMCPPickerHint"
                : "profileKnowledgePickerHint",
          )}
          closeLabel={t("close")}
          busy={busy}
          portalContainer={portalContainer}
          actions={
            <>
              {kind === "skill" ? (
                <Button size="sm" disabled={busy} onClick={() => setUploading(true)}>
                  <Plus size={14} />
                  {t("resourcesSkillUpload")}
                </Button>
              ) : kind === "mcp" ? (
                <Button size="sm" disabled={busy} onClick={() => setCreating(true)}>
                  <Plus size={15} />
                  {t("resourcesMCPCreateTitle")}
                </Button>
              ) : null}
              <LinkButton
                size="sm"
                variant="linkColor"
                href={kind === "skill" ? "#/skills" : kind === "mcp" ? "#/connectors" : "#/knowledge-bases"}
              >
                {t(
                  kind === "skill"
                    ? "profileManageSkills"
                    : kind === "mcp"
                      ? "profileManageMCP"
                      : "profileManageKnowledge",
                )}
              </LinkButton>
            </>
          }
          footer={
            <>
              <Button size="sm" disabled={busy} onClick={onClose}>
                {t(picker.market ? "close" : "cancel")}
              </Button>
              {!picker.market ? (
                <Button
                  size="sm"
                  variant="primary"
                  loading={picker.busy === "library"}
                  disabled={busy || !selected.length || Boolean(picker.libraryError)}
                  onClick={() => void finish(picker.add(selected))}
                >
                  {title}
                </Button>
              ) : null}
            </>
          }
        >
          {kind !== "knowledge" ? (
            <SegmentedControl
              variant="underline"
              label={title}
              value={picker.market ? "market" : "library"}
              onChange={(value) => {
                picker.setMarket(value === "market");
                picker.setSearch("");
              }}
              options={[
                { value: "library", label: t("profileResourceLibrary") },
                { value: "market", label: marketLabel },
              ]}
            />
          ) : null}
          {kind !== "knowledge" ? (
            <TextInput
              type="search"
              value={picker.search}
              aria-label={searchLabel}
              placeholder={searchLabel}
              onChange={(event) => picker.setSearch(event.currentTarget.value)}
            />
          ) : null}
          {failure ? (
            <div className="form-error" role="alert">
              {failure}
              {loadError ? (
                <Button
                  size="sm"
                  onClick={() => {
                    void picker.retryLibrary();
                    void marketQuery.refetch();
                  }}
                >
                  {t("retry")}
                </Button>
              ) : null}
            </div>
          ) : null}
          <ResourcePickerList>
            {!picker.market ? (
              <>
                {picker.libraryLoading ? <p role="status">{t("loading")}</p> : null}
                {visibleLibrary.map((item) => {
                  const installed = picker.installedNames.has(item.name);
                  return (
                    <ResourcePickerRow selectable key={item.name}>
                      <Checkbox
                        checked={installed || selected.includes(item.name)}
                        disabled={busy || installed || Boolean(picker.libraryError)}
                        onCheckedChange={(checked) => select(item.name, checked === true)}
                        aria-label={item.label}
                      />
                      <ResourcePickerCopy
                        name={item.label + (installed ? ` · ${t("profileResourceEnabled")}` : "")}
                        description={item.description}
                      />
                    </ResourcePickerRow>
                  );
                })}
                {!visibleLibrary.length && !picker.libraryLoading && !picker.libraryError && kind !== "knowledge" ? (
                  <p>{t("profileResourceLibraryEmpty")}</p>
                ) : null}
              </>
            ) : null}
            {picker.market || kind === "knowledge" ? (
              <>
                {!authenticated ? (
                  <ResourcePickerActions>
                    <span>
                      {t(
                        kind === "skill"
                          ? "iaSkillsMarketLoginRequired"
                          : kind === "mcp"
                            ? "iaMCPMarketLoginRequired"
                            : "resourcesKnowledgeBasesLoginRequired",
                      )}
                    </span>
                    <Button onClick={() => onLogin?.()}>{t("resourcesKnowledgeBasesLogin")}</Button>
                  </ResourcePickerActions>
                ) : (
                  <>
                    {marketQuery.isFetching && !marketQuery.isFetchingNextPage ? (
                      <p role="status">{t("loading")}</p>
                    ) : null}
                    {remoteRows.map((item) => (
                      <ResourcePickerRow key={item.id}>
                        {kind === "knowledge" ? (
                          <Checkbox disabled checked={false} aria-label={item.name} />
                        ) : (
                          <ResourceFeaturedIcon>
                            {kind === "skill" ? <SidebarPuzzlePiece02Icon size={16} /> : <SidebarMcpIcon size={16} />}
                          </ResourceFeaturedIcon>
                        )}
                        <ResourcePickerCopy name={item.name} description={item.description} onOpen={item.open} />
                        <Button
                          size="sm"
                          variant={kind === "knowledge" ? "secondaryGray" : "primary"}
                          loading={picker.busy === item.id}
                          disabled={
                            busy ||
                            item.installed ||
                            item.unavailable ||
                            picker.libraryLoading ||
                            Boolean(picker.libraryError)
                          }
                          onClick={() => void finish(item.add())}
                        >
                          {t(
                            item.installed
                              ? "profileResourceEnabled"
                              : item.unavailable
                                ? "resourcesKnowledgeBaseUnavailable"
                                : kind === "knowledge"
                                  ? "profileKnowledgeConnectEnable"
                                  : "resourcesSkillRemoteInstallAction",
                          )}
                        </Button>
                      </ResourcePickerRow>
                    ))}
                    {!marketQuery.isFetching &&
                    !marketQuery.error &&
                    !remoteRows.length &&
                    (picker.market || !visibleLibrary.length) ? (
                      <p>{t("workspaceSearchNoResults")}</p>
                    ) : null}
                    {marketQuery.hasNextPage ? (
                      <Button
                        size="sm"
                        disabled={marketQuery.isFetching}
                        onClick={() => void marketQuery.fetchNextPage()}
                      >
                        {t("resourceLoadMore")}
                      </Button>
                    ) : null}
                  </>
                )}
              </>
            ) : null}
          </ResourcePickerList>
        </ResourcePicker>
      </DialogRoot>
      {preview ? (
        <SkillMarketPreview
          skill={preview}
          t={t}
          busy={busy}
          installed={picker.isSkillEnabled(preview)}
          installDisabled={picker.libraryLoading || Boolean(picker.libraryError)}
          error={failure}
          onClose={() => setPreview(null)}
          onAfterClose={() => previewTrigger.current?.focus()}
          onInstall={() => void finish(picker.installSkill(preview))}
          portalContainer={portalContainer}
        />
      ) : null}
      <SkillUploadDialog
        open={uploading}
        zipOnly
        onOpenChange={setUploading}
        busy={busy}
        error={failure}
        installedSkills={picker.skills}
        onSubmit={(file) => finish(picker.upload(file))}
        remoteInstallBusy=""
        remoteInstallError=""
        remoteSkills={[]}
        remoteSkillsError=""
        remoteSkillsHasMore={false}
        remoteSkillsLoading={false}
        remoteSkillsLoadingMore={false}
        remoteSkillsSearch=""
        t={t}
      />
      <MCPServerCreateDialog
        open={creating}
        manualOnly
        onOpenChange={setCreating}
        busy={busy}
        createError={failure}
        initialDocument=""
        onCreate={(payload) => finish(picker.createMCP(payload))}
        remote={{
          error: "",
          hasMore: false,
          installBusy: "",
          installedServers: [],
          items: [],
          loading: false,
          loadingMore: false,
          search: "",
        }}
        t={t}
      />
    </>
  );
}
