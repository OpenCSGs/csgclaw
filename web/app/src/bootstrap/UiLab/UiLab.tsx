import { useEffect, useState } from "react";
import {
  Button,
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

export function UiLab() {
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
        <main className={styles.page}>
          <PanelHeader>
            <h1>CSGClaw UI Lab</h1>
            <SegmentedControl
              value={theme}
              onChange={setTheme}
              label="Preview theme"
              options={[
                { value: "light", label: "Light" },
                { value: "dark", label: "Dark" },
              ]}
            />
          </PanelHeader>
          <p className={styles.intro}>
            Production components, semantic tokens and keyboard interactions. No backend or account is required.
          </p>
          <section className={styles.card}>
            <h2>Turn token consumption</h2>
            {["qwen3.7-plus", "auto", "a-very-long-selected-model-name-for-narrow-screen-layout"].map((model) => (
              <div key={model} className="message-row">
                <div className="message-card">
                  <p>这是一轮已完成的回复。</p>
                  <ConversationMessageActions
                    className="message-hover-actions"
                    content="这是一轮已完成的回复。"
                    t={createTranslator("zh")}
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
          <section className={styles.card}>
            <h2>Buttons and fields</h2>
            <div className={styles.row}>
              <Button variant="primary">Primary</Button>
              <Button variant="secondaryGray">Secondary</Button>
              <Button variant="danger">Danger</Button>
              <Button disabled>Disabled</Button>
              <Button loading>Loading</Button>
              <LinkButton href="#selection-controls">View selection controls</LinkButton>
            </div>
            <div className={styles.grid}>
              <Field label="Name" required hint="Use a short display name" error={error ? "Enter a name" : undefined}>
                <TextInput placeholder="Example" />
              </Field>
              <Field label="Disabled input">
                <TextInput disabled value="Read only" />
              </Field>
              <Field label="Instructions">
                <TextArea defaultValue="A multiline field with its own component styles." />
              </Field>
            </div>
            <label className={styles.row}>
              <input type="checkbox" checked={error} onChange={(event) => setError(event.target.checked)} />
              Show validation errors
            </label>
            <FieldHelpTooltip
              summary="Field help"
              detail="This is the shared tooltip. It opens with pointer hover or keyboard focus and follows the viewport."
            />
          </section>
          <section className={styles.card}>
            <h2>Workspace style boundary</h2>
            <div className="profile-section">
              <Field label="Workspace input" error={error ? "Enter a value" : undefined}>
                <TextInput placeholder="Component styles inside legacy workspace CSS" />
              </Field>
              <div className={styles.row}>
                <Button variant="primary" disabled>
                  Disabled primary
                </Button>
                <Button variant="primary" loading>
                  Saving
                </Button>
                <Checkbox aria-label="Disabled checkbox" checked disabled />
                <Switch aria-label="Disabled switch" checked disabled />
              </div>
            </div>
            <ConversationSection
              title="Conversation button styles"
              items={
                showDemoConversation
                  ? [{ id: "ui-lab-room", title: "UI Lab conversation", members: [], messages: [] }]
                  : []
              }
              activeConversationId="ui-lab-room"
              currentUserID="ui-lab-user"
              usersById={new Map()}
              locale="en"
              t={createTranslator("en")}
              onSelect={() => {}}
              onDelete={() => setShowDemoConversation(false)}
            />
          </section>
          <section className={styles.card} id="selection-controls">
            <h2>Selection controls</h2>
            <div className={styles.grid}>
              <Field label="Sort order">
                <Select
                  value={sort}
                  onValueChange={setSort}
                  options={[
                    { value: "", label: "None" },
                    { value: "recent", label: "Most recent" },
                    { value: "name", label: "Name" },
                  ]}
                />
              </Field>
              <Field
                label="Search picker"
                hint="Try arrows, Enter, Escape and Tab"
                error={error ? "Choose a model" : undefined}
              >
                <Combobox
                  value={value}
                  onValueChange={setValue}
                  options={options}
                  searchPlaceholder="Search models"
                  emptyLabel="No matching models"
                />
              </Field>
              <Field label="Disabled picker">
                <Combobox disabled value={value} options={options} searchPlaceholder="Search disabled picker" />
              </Field>
              <Field label="Model">
                <ModelPicker
                  providerID="opencsg"
                  value={model}
                  models={["auto", "qwen-3.5", "deepseek-v4", "a-model-with-a-long-name"]}
                  onChange={setModel}
                  placeholder="Choose a model"
                  t={createTranslator("en")}
                />
              </Field>
            </div>
            <output className={styles.output}>Selected: {value}</output>
            <Button variant="secondaryGray" onClick={() => setValue("")}>
              Clear selection
            </Button>
          </section>
          <section className={styles.card}>
            <h2>Reasoning effort</h2>
            <div className={styles.reasoningPreview}>
              <ReasoningEffortPicker
                value={reasoning}
                agentEffort="high"
                onValueChange={setReasoning}
                onValueCommit={setReasoning}
                t={createTranslator("zh")}
              />
            </div>
          </section>
          <section className={styles.card}>
            <PanelHeader>
              <h2>Dialog and empty state</h2>
              <Button onClick={() => setDialog(true)}>Open nested dialog</Button>
            </PanelHeader>
            <EmptyState
              title="No items yet"
              description="Add an item to see it here."
              actions={<Button variant="secondaryGray">Add item</Button>}
            />
          </section>
          <DialogRoot open={dialog} onOpenChange={setDialog}>
            <DialogContent aria-describedby={undefined}>
              <DialogHeader>
                <DialogTitle>Nested selection</DialogTitle>
                <DialogCloseButton label="Close dialog" />
              </DialogHeader>
              <DialogBody>
                <Field label="Dialog model" required>
                  <Combobox
                    value={value}
                    onValueChange={setValue}
                    options={options}
                    searchPlaceholder="Search dialog models"
                    emptyLabel="No matches"
                    required
                    name="model"
                  />
                </Field>
                <Button>Next dialog field</Button>
              </DialogBody>
            </DialogContent>
          </DialogRoot>
        </main>
      </div>
    </TooltipProvider>
  );
}
