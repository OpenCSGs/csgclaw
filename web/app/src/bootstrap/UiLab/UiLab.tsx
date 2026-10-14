import { useEffect, useState } from "react";
import {
  Button,
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
  TextArea,
  TextInput,
  TooltipProvider,
} from "@/components/ui";
import { ModelPicker } from "@/components/business/AgentModelFields";
import { FieldHelpTooltip } from "@/components/business/ProfileControls";
import { createTranslator } from "@/shared/i18n";
import type { ResolvedThemeMode } from "@/shared/theme/theme";
import styles from "./UiLab.module.css";

const options = Array.from({ length: 180 }, (_, index) => ({
  value: `model-${index + 1}`,
  label:
    index === 2
      ? "A very long model name that should remain readable inside a narrow picker"
      : `Model ${String(index + 1).padStart(3, "0")}`,
  disabled: index === 1,
}));

export function UiLab() {
  const [theme, setTheme] = useState<ResolvedThemeMode>("light");
  const [value, setValue] = useState("model-1");
  const [sort, setSort] = useState("recent");
  const [model, setModel] = useState("qwen-3.5");
  const [dialog, setDialog] = useState(false);
  const [error, setError] = useState(false);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
  }, [theme]);
  return (
    <TooltipProvider>
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
          <h2>Buttons and fields</h2>
          <div className={styles.row}>
            <Button variant="primary">Primary</Button>
            <Button variant="secondaryGray">Secondary</Button>
            <Button variant="danger">Danger</Button>
            <Button disabled>Disabled</Button>
            <Button loading>Loading</Button>
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
            <Field label="Search picker" hint="Try arrows, Enter, Escape and Tab">
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
                value={model}
                models={["qwen-3.5", "deepseek-v4", "a-model-with-a-long-name"]}
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
    </TooltipProvider>
  );
}
