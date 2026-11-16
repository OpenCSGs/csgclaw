import canvasStyles from "./LabPreviewCanvas.module.css";
import resetStyles from "./LabOptions.module.css";
import { LabComponentHeading } from "./LabComponentHeading";
import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import messageLayoutStyles from "@/components/business/ConversationPane/MessageLayout.module.css";
import { DropdownPreview } from "./DropdownFoundations/DropdownPreview";
import { UiLabCatalog } from "./UiLabCatalog";
import { labCatalogId, labGroups, type LabSectionId } from "./UiLabNavigation/navigation";
import { LabView } from "./labViewContext";
import { UiLabPanel } from "./UiLabPanel";
import { ApplicationFoundations } from "./ApplicationFoundations/ApplicationFoundations";
import { applicationSections } from "./ApplicationFoundations/registry";
import { FooterFoundations } from "./FooterFoundations";
import {
  TabsFoundations,
  PaginationFoundations,
  StepsFoundations,
} from "./NavigationFoundations/NavigationFoundations";
import { LabLocaleProvider, useLabTranslation } from "./LabLocale";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import { Plus } from "lucide-react";
import {
  Button,
  IconButton,
  LinkButton,
  Checkbox,
  Combobox,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  EmptyState,
  Field,
  PanelHeader,
  SegmentedControl,
  Select,
  Switch,
  TextArea,
  TextInput,
  TooltipProvider,
} from "@/components/ui";
import { ModelPicker } from "@/components/business/AgentModelFields";
import { CloudPrivacyDialog } from "@/components/business/CloudPrivacyDialog";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { cloudAgentAvatar } from "@/models/cloudAgents";
import { CLOUD_AGENT_AVATARS } from "@/shared/avatar";
import { ReasoningEffortPicker } from "@/components/business/ReasoningEffortPicker";
import { FieldHelpTooltip } from "@/components/business/ProfileControls";
import { ConversationSection } from "@/components/business/ConversationWorkspace/ConversationSection";
import { ConversationMessageActions } from "@/components/business/ConversationPane/ConversationMessageActions";
import "@/components/business/ConversationPane/ConversationThreads.css";
import { createTranslator } from "@/shared/i18n";
import type { ResolvedThemeMode } from "@/shared/theme/theme";
import styles from "./UiLab.module.css";
import { ColorFoundations } from "./ColorFoundations";
import { TypographyFoundations } from "./TypographyFoundations";
import { ShadowFoundations } from "./ShadowFoundations";
import { LayoutFoundations } from "./LayoutFoundations";
import { ButtonFoundations } from "./ButtonFoundations";
import { TagFoundations } from "./TagFoundations";
import { InputFoundations } from "./InputFoundations";
import { ToggleFoundations } from "./ToggleFoundations";
import { CheckboxFoundations } from "./CheckboxFoundations";
import { TooltipFoundations } from "./TooltipFoundations";
import { SliderFoundations } from "./SliderFoundations";
import { ProgressFoundations } from "./ProgressFoundations";
import { UiLabNavigation, sectionFromHash } from "./UiLabNavigation";
import "@/pages/WorkspacePage/components/WorkspaceModals/WorkspaceModals.css";

const options = Array.from({ length: 180 }, (_, index) => ({
  value: `model-${index + 1}`,
  label:
    index === 2
      ? "A_very_long_model_identifier_that_should_remain_readable_inside_a_narrow_picker_without_horizontal_scrolling"
      : `Model ${String(index + 1).padStart(3, "0")}`,
  disabled: index === 1,
  separatorAfter: index === 0,
}));

function subscribeToHash(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}
const getHash = () => window.location.hash;
const sectionIds: LabSectionId[] = labGroups.flatMap((group) => group.items.map(({ id }) => id));
const designItems = labGroups.flatMap((group) => [...group.items]).filter(({ id }) => id.endsWith("-foundations"));
const batchSize = 4;

function UiLabContent() {
  const { text: labText, locale, setLocale } = useLabTranslation();

  const hash = useSyncExternalStore(subscribeToHash, getHash, () => "");
  const destination = sectionFromHash(hash.replace(/\/design$/, ""));
  const designView = destination !== labCatalogId && hash.endsWith("/design");
  const contentRef = useRef<HTMLElement>(null);
  const loadMoreRef = useRef<HTMLButtonElement>(null);
  const [active, setActive] = useState<LabSectionId>(destination);
  const [loaded, setLoaded] = useState(batchSize);
  const browsing = destination !== labCatalogId && !designView;
  const designIndex = designView ? designItems.findIndex(({ id }) => id === destination) : -1;
  const adjacentDesigns = designIndex < 0 ? [] : [designItems[designIndex - 1], designItems[designIndex + 1]];
  const visibleCount = Math.max(loaded, sectionIds.indexOf(destination) + 1);
  const hasMore = browsing && visibleCount < sectionIds.length;

  useLayoutEffect(() => {
    const content = contentRef.current;
    if (!content) return;
    setActive(destination);
    setLoaded((count) => Math.max(count, sectionIds.indexOf(destination) + 1));
    if (destination === labCatalogId || designView) {
      content.scrollTop = 0;
    } else {
      document.getElementById(destination)?.scrollIntoView({ block: "start" });
    }
    content.focus({ preventScroll: true });
  }, [destination, designView]);

  const loadMore = useCallback(() => {
    setLoaded((count) => Math.min(Math.max(count, visibleCount) + batchSize, sectionIds.length));
  }, [visibleCount]);

  useEffect(() => {
    const sentinel = loadMoreRef.current;
    if (!hasMore || !sentinel || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) loadMore();
      },
      { root: contentRef.current, rootMargin: "0px 0px 400px" },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, loadMore]);

  function updateActiveSection() {
    const content = contentRef.current;
    if (!browsing || !content) return;
    if (!hasMore && content.scrollHeight - content.clientHeight - content.scrollTop <= 2) {
      setActive(sectionIds[sectionIds.length - 1]);
      return;
    }
    const top = content.getBoundingClientRect().top + 24;
    let current = sectionIds[0];
    for (const id of sectionIds.slice(0, visibleCount)) {
      const panel = document.getElementById(id);
      if (!panel || panel.getBoundingClientRect().top > top) break;
      current = id;
    }
    setActive(current);
  }

  function isVisible(id: LabSectionId) {
    return designView ? id === destination : browsing && sectionIds.indexOf(id) < visibleCount;
  }

  const [theme, setTheme] = useState<ResolvedThemeMode>("light");
  const [value, setValue] = useState("model-1");
  const [workspaceInput, setWorkspaceInput] = useState("");
  const [sort, setSort] = useState("recent");
  const [model, setModel] = useState("qwen-3.5");
  const [reasoning, setReasoning] = useState("");
  const [dialog, setDialog] = useState(false);
  const [cloudPrivacyDialog, setCloudPrivacyDialog] = useState(false);
  const [error, setError] = useState(false);
  const [showDemoConversation, setShowDemoConversation] = useState(true);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
  }, [theme]);
  return (
    <TooltipProvider>
      <div className={styles.viewport}>
        <UiLabNavigation active={active} />
        <main
          className={styles.content}
          ref={contentRef}
          onScroll={updateActiveSection}
          tabIndex={-1}
          id="ui-lab-content"
        >
          <div className={styles.page}>
            <PanelHeader>
              <h1>{labText("CSGClaw UI Lab")}</h1>
              <div className={styles.preferences}>
                <SegmentedControl
                  label={labText("语言")}
                  value={locale}
                  onChange={(value) => setLocale(value === "en" ? "en" : "zh")}
                  options={[
                    { value: "zh", label: "中文" },
                    { value: "en", label: "English" },
                  ]}
                />
                <SegmentedControl
                  value={theme}
                  onChange={setTheme}
                  label={labText("Preview theme")}
                  options={[
                    { value: "light", label: labText("Light") },
                    { value: "dark", label: labText("Dark") },
                  ]}
                />
              </div>
            </PanelHeader>
            {browsing && (
              <a className={styles.catalogBack} href={`#${labCatalogId}`}>
                ← {labText("组件目录")}
              </a>
            )}
            <UiLabPanel id={labCatalogId} active={destination}>
              <UiLabCatalog />
            </UiLabPanel>
            {designView && (
              <a className={styles.catalogBack} href={`#${destination}`}>
                ← {labText("返回交互试用")}
              </a>
            )}
            <LabView.Provider value={designView ? "design" : "preview"}>
              <UiLabPanel id="design-foundations" active={destination} visible={isVisible("design-foundations")}>
                <ColorFoundations />
              </UiLabPanel>
              <UiLabPanel
                id="typography-foundations"
                active={destination}
                visible={isVisible("typography-foundations")}
              >
                <TypographyFoundations />
              </UiLabPanel>
              <UiLabPanel id="shadow-foundations" active={destination} visible={isVisible("shadow-foundations")}>
                <ShadowFoundations />
              </UiLabPanel>
              <UiLabPanel id="layout-foundations" active={destination} visible={isVisible("layout-foundations")}>
                <LayoutFoundations />
              </UiLabPanel>
              <UiLabPanel id="button-foundations" active={destination} visible={isVisible("button-foundations")}>
                <ButtonFoundations />
              </UiLabPanel>
              <UiLabPanel id="tag-foundations" active={destination} visible={isVisible("tag-foundations")}>
                <TagFoundations />
              </UiLabPanel>
              <UiLabPanel id="input-foundations" active={destination} visible={isVisible("input-foundations")}>
                <InputFoundations />
              </UiLabPanel>
              <UiLabPanel id="dropdown-foundations" active={destination} visible={isVisible("dropdown-foundations")}>
                <section className={styles.card} id="dropdown-foundations" aria-labelledby="dropdown-title">
                  <LabComponentHeading sectionId="dropdown-foundations" titleId="dropdown-title">
                    {labText("Dropdowns · 下拉菜单")}
                  </LabComponentHeading>
                  <p lang={locale === "zh" ? "zh-CN" : "en"}>
                    {labText(
                      "Dropdowns are used to group together actions in a subview. They’re useful for hiding menus or when you have multiple actions that cannot be separate buttons.",
                    )}
                  </p>
                  <DropdownPreview />
                </section>
              </UiLabPanel>
              <UiLabPanel id="toggle-foundations" active={destination} visible={isVisible("toggle-foundations")}>
                <ToggleFoundations />
              </UiLabPanel>
              <UiLabPanel id="checkbox-foundations" active={destination} visible={isVisible("checkbox-foundations")}>
                <CheckboxFoundations />
              </UiLabPanel>
              <UiLabPanel id="slider-foundations" active={destination} visible={isVisible("slider-foundations")}>
                <SliderFoundations />
              </UiLabPanel>
              <UiLabPanel id="tooltip-foundations" active={destination} visible={isVisible("tooltip-foundations")}>
                <TooltipFoundations />
              </UiLabPanel>
              <UiLabPanel id="progress-foundations" active={destination} visible={isVisible("progress-foundations")}>
                <ProgressFoundations />
              </UiLabPanel>
              <UiLabPanel id="tabs-foundations" active={destination} visible={isVisible("tabs-foundations")}>
                <TabsFoundations />
              </UiLabPanel>
              <UiLabPanel id="footer-foundations" active={destination} visible={isVisible("footer-foundations")}>
                <FooterFoundations />
              </UiLabPanel>
              <UiLabPanel
                id="pagination-foundations"
                active={destination}
                visible={isVisible("pagination-foundations")}
              >
                <PaginationFoundations />
              </UiLabPanel>
              <UiLabPanel id="steps-foundations" active={destination} visible={isVisible("steps-foundations")}>
                <StepsFoundations />
              </UiLabPanel>
              {applicationSections.map(({ key }) => (
                <UiLabPanel
                  key={key}
                  id={`${key}-foundations`}
                  active={destination}
                  visible={isVisible(`${key}-foundations`)}
                >
                  <ApplicationFoundations kind={key} theme={theme} />
                </UiLabPanel>
              ))}
              <UiLabPanel id="token-consumption" active={destination} visible={isVisible("token-consumption")}>
                <section className={styles.card} id="token-consumption">
                  <h2>{labText("Turn token consumption")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    {["qwen3.7-plus", "auto", "a-very-long-selected-model-name-for-narrow-screen-layout"].map((model) => (
                      <div key={model} className={`message-row ${messageLayoutStyles["message-row"]}`}>
                        <div className={`message-card ${messageLayoutStyles["message-card"]}`}>
                          <p>{labText("这是一轮已完成的回复。")}</p>
                          <ConversationMessageActions
                            className="message-hover-actions"
                            content="这是一轮已完成的回复。"
                            t={createTranslator(locale)}
                            metadata={{
                              csgclaw: {
                                turn_progress: {
                                  status: "succeeded",
                                  usage: {
                                    known: true,
                                    model_id: model,
                                    input_tokens: 1000,
                                    output_tokens: 220,
                                    total_tokens: 1220,
                                  },
                                },
                              },
                            }}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                </section>
              </UiLabPanel>
              <UiLabPanel id="secondary-gray" active={destination} visible={isVisible("secondary-gray")}>
                <section className={styles.card} id="secondary-gray">
                  <h2>{labText("Secondary gray · Figma")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <p className={styles.intro}>
                      {labText("Default, leading icon, disabled, loading, icon only and navigation.")}
                    </p>
                    {(["sm", "md", "lg", "xl", "2xl"] as const).map((size) => (
                      <div key={size} className={styles.row} role="group" aria-label={`Secondary gray ${size}`}>
                        <span>{size}</span>
                        <Button size={size}>{labText("登录账号")}</Button>
                        <Button size={size}>
                          <Plus />
                          {labText("新建智能体")}
                        </Button>
                        <Button size={size} disabled>
                          {labText("登录账号")}
                        </Button>
                        <Button size={size} loading loadingLabel={`Saving ${size}`}>
                          {labText("登录账号")}
                        </Button>
                        <IconButton size={size} icon={<Plus />} label={`Add ${size}`} />
                        <LinkButton size={size} href="#selection-controls">
                          {labText("View items")}
                        </LinkButton>
                      </div>
                    ))}
                  </div>
                </section>
              </UiLabPanel>
              <UiLabPanel id="buttons-and-fields" active={destination} visible={isVisible("buttons-and-fields")}>
                <section className={styles.card} id="buttons-and-fields">
                  <h2>{labText("Buttons and fields")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <div className={styles.row}>
                      <Button variant="primary">{labText("Primary")}</Button>
                      <Button variant="secondaryGray">{labText("Secondary")}</Button>
                      <Button variant="danger">{labText("Danger")}</Button>
                      <Button disabled>{labText("Disabled")}</Button>
                      <Button loading>{labText("Loading")}</Button>
                      <LinkButton href="#selection-controls">{labText("View selection controls")}</LinkButton>
                    </div>
                    <div className={styles.grid}>
                      <Field
                        label={labText("Name")}
                        required
                        hint={labText("Use a short display name")}
                        error={error ? labText("Enter a name") : undefined}
                      >
                        <TextInput placeholder={labText("Example")} />
                      </Field>
                      <Field label={labText("Disabled input")}>
                        <TextInput disabled value={labText("Read only")} />
                      </Field>
                      <Field label={labText("Instructions")}>
                        <TextArea defaultValue={labText("A multiline field with its own component styles.")} />
                      </Field>
                    </div>
                    <label className={styles.row}>
                      <input type="checkbox" checked={error} onChange={(event) => setError(event.target.checked)} />
                      {labText("Show validation errors")}
                    </label>
                    <FieldHelpTooltip
                      summary={labText("Field help")}
                      detail={labText(
                        "This is the shared tooltip. It opens with pointer hover or keyboard focus and follows the viewport.",
                      )}
                    />
                  </div>
                </section>
              </UiLabPanel>
              <UiLabPanel id="workspace-boundary" active={destination} visible={isVisible("workspace-boundary")}>
                <section className={styles.card} id="workspace-boundary">
                  <h2>{labText("Workspace style boundary")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <div className={resetStyles.resetRow}>
                      <Button
                        size="sm"
                        onClick={() => {
                          setWorkspaceInput("");
                          setShowDemoConversation(true);
                        }}
                      >
                        {labText("重置示例")}
                      </Button>
                    </div>
                    <div className={`profile-section ${profileFormLayoutStyles["profile-section"]}`}>
                      <Field label={labText("Workspace input")} error={error ? labText("Enter a value") : undefined}>
                        <TextInput
                          placeholder={labText("Component styles inside legacy workspace CSS")}
                          value={workspaceInput}
                          onChange={(event) => setWorkspaceInput(event.target.value)}
                        />
                      </Field>
                      <div className={styles.row}>
                        <Button variant="primary" disabled>
                          {labText("Disabled primary")}
                        </Button>
                        <Button variant="primary" loading>
                          {labText("Saving")}
                        </Button>
                        <Checkbox aria-label={labText("Disabled checkbox")} checked disabled />
                        <Switch aria-label={labText("Disabled switch")} checked disabled />
                      </div>
                    </div>
                    <ConversationSection
                      title={labText("Conversation button styles")}
                      items={
                        showDemoConversation
                          ? [{ id: "ui-lab-room", title: "UI Lab conversation", members: [], messages: [] }]
                          : []
                      }
                      activeConversationId="ui-lab-room"
                      currentUserID="ui-lab-user"
                      usersById={new Map()}
                      locale={locale}
                      t={createTranslator(locale)}
                      onSelect={() => {}}
                      onDelete={() => setShowDemoConversation(false)}
                    />
                  </div>
                </section>
              </UiLabPanel>
              <UiLabPanel id="selection-controls" active={destination} visible={isVisible("selection-controls")}>
                <section className={styles.card} id="selection-controls">
                  <h2>{labText("Selection controls")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <div className={resetStyles.resetRow}>
                      <Button
                        size="sm"
                        onClick={() => {
                          setSort("recent");
                          setValue("model-1");
                          setModel("qwen-3.5");
                        }}
                      >
                        {labText("重置示例")}
                      </Button>
                    </div>
                    <div className={styles.grid}>
                      <Field label={labText("Sort order")}>
                        <Select
                          value={sort}
                          onValueChange={setSort}
                          options={[
                            { value: "", label: labText("None") },
                            { value: "recent", label: labText("Most recent") },
                            { value: "name", label: labText("Name") },
                          ]}
                        />
                      </Field>
                      <Field
                        label={labText("Search picker")}
                        hint={labText("Try arrows, Enter, Escape and Tab")}
                        error={error ? labText("Choose a model") : undefined}
                      >
                        <Combobox
                          value={value}
                          onValueChange={setValue}
                          options={options}
                          searchPlaceholder={labText("Search models")}
                          emptyLabel={labText("No matching models")}
                        />
                      </Field>
                      <Field label={labText("Disabled picker")}>
                        <Combobox
                          disabled
                          value={value}
                          options={options}
                          searchPlaceholder={labText("Search disabled picker")}
                        />
                      </Field>
                      <Field label={labText("Model")}>
                        <ModelPicker
                          providerID="opencsg"
                          value={model}
                          models={["auto", "qwen-3.5", "deepseek-v4", "a-model-with-a-long-name"]}
                          onChange={setModel}
                          placeholder={labText("Choose a model")}
                          t={createTranslator(locale)}
                        />
                      </Field>
                    </div>
                    <output className={styles.output}>
                      {labText("Selected: ")}
                      {value}
                    </output>
                    <Button variant="secondaryGray" onClick={() => setValue("")}>
                      {labText("Clear selection")}
                    </Button>
                  </div>
                </section>
              </UiLabPanel>
              <UiLabPanel id="reasoning-effort" active={destination} visible={isVisible("reasoning-effort")}>
                <section className={styles.card} id="reasoning-effort">
                  <h2>{labText("Reasoning effort")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <div className={resetStyles.resetRow}>
                      <Button size="sm" onClick={() => setReasoning("")}>
                        {labText("重置示例")}
                      </Button>
                    </div>
                    <div className={styles.reasoningPreview}>
                      <ReasoningEffortPicker
                        value={reasoning}
                        agentEffort="high"
                        onValueChange={setReasoning}
                        onValueCommit={setReasoning}
                        t={createTranslator(locale)}
                      />
                    </div>
                  </div>
                </section>
              </UiLabPanel>
              <UiLabPanel id="dialogs-and-empty" active={destination} visible={isVisible("dialogs-and-empty")}>
                <section className={styles.card} id="dialogs-and-empty">
                  <h2>{labText("Dialog and empty state")}</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <Button onClick={() => setDialog(true)}>{labText("Open nested dialog")}</Button>
                    <EmptyState
                      title={labText("No items yet")}
                      description={labText("Add an item to see it here.")}
                      actions={<Button variant="secondaryGray">{labText("Add item")}</Button>}
                    />
                  </div>
                </section>
                <section className={styles.card}>
                  <h2>Cloud agent confirmation</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <Button onClick={() => setCloudPrivacyDialog(true)}>Preview cloud privacy dialog</Button>
                  </div>
                </section>
                <section className={styles.card}>
                  <h2>Cloud agent type avatars</h2>
                  <div className={canvasStyles.previewCanvas}>
                    <div className={styles.row}>
                      {Object.keys(CLOUD_AGENT_AVATARS).map((type) => (
                        <div key={type} className={styles.cloudAvatarPreview}>
                          <AgentAvatarContent avatar={cloudAgentAvatar(type)} alt={type} />
                          <span>{type}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </section>
                {cloudPrivacyDialog ? (
                  <CloudPrivacyDialog
                    agentName="气象运维助手"
                    t={createTranslator(locale)}
                    confirmation={{
                      onCancel: () => setCloudPrivacyDialog(false),
                      onConfirm: () => setCloudPrivacyDialog(false),
                    }}
                  />
                ) : null}
              </UiLabPanel>
            </LabView.Provider>
            {adjacentDesigns.length > 0 && (
              <nav className={styles.designPager} aria-label={labText("相邻组件")}>
                {adjacentDesigns.map((item, index) =>
                  item ? (
                    <a
                      key={item.id}
                      className={styles.designPagerLink}
                      data-direction={index === 0 ? "previous" : "next"}
                      href={`#${item.id}/design`}
                      rel={index === 0 ? "prev" : "next"}
                    >
                      <span>{labText(index === 0 ? "上一个组件" : "下一个组件")}</span>
                      <strong>{locale === "zh" && "zh" in item ? item.zh : labText(item.label)}</strong>
                    </a>
                  ) : null,
                )}
              </nav>
            )}
            {hasMore && (
              <Button ref={loadMoreRef} onClick={loadMore}>
                {labText("加载更多组件")}
              </Button>
            )}
            <DialogRoot open={dialog} onOpenChange={setDialog}>
              <DialogContent aria-describedby={undefined}>
                <DialogHeader>
                  <DialogTitle>{labText("Nested selection")}</DialogTitle>
                  <DialogCloseButton label={labText("Close dialog")} />
                </DialogHeader>
                <DialogBody>
                  <Field label={labText("Dialog model")} required>
                    <Combobox
                      value={value}
                      onValueChange={setValue}
                      options={options}
                      searchPlaceholder={labText("Search dialog models")}
                      emptyLabel={labText("No matches")}
                      required
                      name="model"
                    />
                  </Field>
                  <Button>{labText("Next dialog field")}</Button>
                </DialogBody>
              </DialogContent>
            </DialogRoot>
          </div>
        </main>
      </div>
    </TooltipProvider>
  );
}

export function UiLab() {
  return (
    <LabLocaleProvider>
      <UiLabContent />
    </LabLocaleProvider>
  );
}
