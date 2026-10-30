import { LabLocaleProvider, useLabTranslation } from "./LabLocale";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
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
import "@/pages/WorkspacePage/components/WorkspaceComponents.css";
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
const getSection = () => sectionFromHash(window.location.hash);

function UiLabContent() {
  const { text: labText, locale, setLocale } = useLabTranslation();

  const active = useSyncExternalStore(subscribeToHash, getSection, () => sectionFromHash(""));
  const contentRef = useRef<HTMLElement>(null);
  const previousSection = useRef(active);
  useEffect(() => {
    if (previousSection.current !== active) {
      contentRef.current?.focus({ preventScroll: true });
      if (contentRef.current) contentRef.current.scrollTop = 0;
      previousSection.current = active;
    }
  }, [active]);
  const [theme, setTheme] = useState<ResolvedThemeMode>("light");
  const [value, setValue] = useState("model-1");
  const [sort, setSort] = useState("recent");
  const [model, setModel] = useState("qwen-3.5");
  const [reasoning, setReasoning] = useState("");
  const [dialog, setDialog] = useState(false);
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
        <main className={styles.content} ref={contentRef} tabIndex={-1} id="ui-lab-content">
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
            <p className={styles.intro}>
              {labText(
                "Production components, semantic tokens and keyboard interactions. No backend or account is required.",
              )}
            </p>
            <p className={styles.intro}>
              {labText("Figma 原始变量名及静态参考文案保留原文；切换语言会保留当前页面和已编辑内容。")}
            </p>
            <div className={styles.panel} hidden={active !== "design-foundations"}>
              <ColorFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "typography-foundations"}>
              <TypographyFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "shadow-foundations"}>
              <ShadowFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "layout-foundations"}>
              <LayoutFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "button-foundations"}>
              <ButtonFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "tag-foundations"}>
              <TagFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "input-foundations"}>
              <InputFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "dropdown-foundations"}>
              <section className={styles.card} id="dropdown-foundations" aria-labelledby="dropdown-title">
                <h2 id="dropdown-title">{labText("Dropdowns · 下拉菜单")}</h2>
                <p lang={locale === "zh" ? "zh-CN" : "en"}>
                  {labText(
                    "Dropdowns are used to group together actions in a subview. They’re useful for hiding menus or when you have multiple actions that cannot be separate buttons.",
                  )}
                </p>
                <p className={styles.intro}>{labText("组件说明已收录，Figma 设计示例与交互演示待补充。")}</p>
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "toggle-foundations"}>
              <ToggleFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "checkbox-foundations"}>
              <CheckboxFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "tooltip-foundations"}>
              <TooltipFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "progress-foundations"}>
              <ProgressFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "slider-foundations"}>
              <SliderFoundations />
            </div>
            <div className={styles.panel} hidden={active !== "token-consumption"}>
              <section className={styles.card} id="token-consumption">
                <h2>{labText("Turn token consumption")}</h2>
                {["qwen3.7-plus", "auto", "a-very-long-selected-model-name-for-narrow-screen-layout"].map((model) => (
                  <div key={model} className="message-row">
                    <div className="message-card">
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
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "secondary-gray"}>
              <section className={styles.card} id="secondary-gray">
                <h2>{labText("Secondary gray · Figma")}</h2>
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
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "buttons-and-fields"}>
              <section className={styles.card} id="buttons-and-fields">
                <h2>{labText("Buttons and fields")}</h2>
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
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "workspace-boundary"}>
              <section className={styles.card} id="workspace-boundary">
                <h2>{labText("Workspace style boundary")}</h2>
                <div className="profile-section">
                  <Field label={labText("Workspace input")} error={error ? labText("Enter a value") : undefined}>
                    <TextInput placeholder={labText("Component styles inside legacy workspace CSS")} />
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
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "selection-controls"}>
              <section className={styles.card} id="selection-controls">
                <h2>{labText("Selection controls")}</h2>
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
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "reasoning-effort"}>
              <section className={styles.card} id="reasoning-effort">
                <h2>{labText("Reasoning effort")}</h2>
                <div className={styles.reasoningPreview}>
                  <ReasoningEffortPicker
                    value={reasoning}
                    agentEffort="high"
                    onValueChange={setReasoning}
                    onValueCommit={setReasoning}
                    t={createTranslator(locale)}
                  />
                </div>
              </section>
            </div>
            <div className={styles.panel} hidden={active !== "dialogs-and-empty"}>
              <section className={styles.card} id="dialogs-and-empty">
                <PanelHeader>
                  <h2>{labText("Dialog and empty state")}</h2>
                  <Button onClick={() => setDialog(true)}>{labText("Open nested dialog")}</Button>
                </PanelHeader>
                <EmptyState
                  title={labText("No items yet")}
                  description={labText("Add an item to see it here.")}
                  actions={<Button variant="secondaryGray">{labText("Add item")}</Button>}
                />
              </section>
            </div>
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
