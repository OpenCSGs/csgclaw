import { useCallback, useRef, useState, type ReactNode } from "react";
import { Check } from "lucide-react";
import {
  Button,
  DialogRoot,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  TextArea,
} from "@/components/ui";
import { AgentCreateProgress, type AgentCreateProgressProps } from "@/components/business/ProfileControls";
import type { AgentCreationResources } from "@/models/agentCreation";
import type { TranslateFn } from "@/models/conversations";
import { AgentCreationResourcesPicker } from "./AgentCreationResourcesPicker";
import { useAgentCreationResourceQueries } from "./useAgentCreationResourceQueries";
import styles from "./AgentCreateWizard.module.css";

type Props = {
  profile: ReactNode;
  onPortalContainer?: (node: HTMLDivElement | null) => void;
  instructions: string;
  onInstructionsChange: (value: string) => void;
  valid: boolean;
  busy: boolean;
  error?: ReactNode;
  progress: AgentCreateProgressProps["progress"];
  authenticated: boolean;
  onRequireAuth: () => boolean;
  onClose: () => void;
  onSave: (resources: AgentCreationResources) => void | Promise<void>;
  t: TranslateFn;
};

export function AgentCreateWizard({
  profile,
  onPortalContainer,
  instructions,
  onInstructionsChange,
  valid,
  busy,
  error,
  progress,
  authenticated,
  onRequireAuth,
  onClose,
  onSave,
  t,
}: Props) {
  const contentRef = useRef<HTMLDivElement | null>(null);
  const handleContentRef = useCallback(
    (node: HTMLDivElement | null) => {
      contentRef.current = node;
      onPortalContainer?.(node);
    },
    [onPortalContainer],
  );
  const [step, setStep] = useState(0);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const goToStep = (next: number) => {
    setStep(next);
    if (panelRef.current) panelRef.current.scrollTop = 0;
  };
  const resourceQueries = useAgentCreationResourceQueries(authenticated);
  const [resources, setResources] = useState<AgentCreationResources>({
    skills: [],
    connectors: [],
    apps: [],
    knowledgeBases: [],
  });
  const steps = [
    { label: t("agentProfileTab"), count: 0 },
    { label: t("agentInstructions"), count: instructions.trim() ? 1 : 0 },
    { label: t("agentProfileSkillsTab"), count: resources.skills.length },
    { label: t("agentAppsTab"), count: resources.connectors.length + resources.apps.length },
    { label: t("resourcesKnowledgeBasesLabel"), count: resources.knowledgeBases.length },
  ];
  const last = step === steps.length - 1;
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={`${styles.dialog} profile-modal agent-modal`}
        ref={handleContentRef}
        onEscapeKeyDown={(event) => {
          if (contentRef.current?.querySelector('[role="dialog"][aria-modal="true"]')) event.preventDefault();
        }}
      >
        <header className={styles.header}>
          <DialogCloseButton className={styles.close} label={t("close")} disabled={busy} />
          <DialogTitle>{t("taskEntryCreateAgent")}</DialogTitle>
          <DialogDescription>{t("createAgentWizardSubtitle")}</DialogDescription>
        </header>
        <div className={styles.body}>
          <nav className={styles.navigation} aria-label={t("taskEntryCreateAgent")}>
            <ol>
              {steps.map((item, index) => (
                <li key={item.label}>
                  <button
                    type="button"
                    aria-label={item.label}
                    aria-current={step === index ? "step" : undefined}
                    disabled={busy}
                    onClick={() => goToStep(index)}
                  >
                    <span className={styles.stepMark} aria-hidden="true">
                      {index === 0 && valid ? <Check size={14} /> : index + 1}
                    </span>
                    <span className={styles.stepLabel} title={item.label}>
                      {item.label}
                    </span>
                    <small className={index === 0 && !valid ? styles.requiredHint : undefined}>
                      {index === 0
                        ? valid
                          ? ""
                          : t("agentWizardRequiredHint")
                        : item.count
                          ? t("agentWizardSelectedCount", { count: item.count })
                          : t("agentWizardNothingSelected")}
                    </small>
                  </button>
                </li>
              ))}
            </ol>
          </nav>
          <div className={styles.panel} ref={panelRef}>
            <fieldset className={styles.fields} disabled={busy}>
              <div hidden={step !== 0}>{profile}</div>
              {step === 1 ? (
                <label className={styles.instructions}>
                  <strong>{t("agentInstructions")}</strong>
                  <TextArea
                    value={instructions}
                    rows={14}
                    placeholder={t("agentInstructionsPlaceholder")}
                    onChange={(event) => onInstructionsChange(event.target.value)}
                  />
                </label>
              ) : null}
              {step >= 2 ? (
                <AgentCreationResourcesPicker
                  key={step}
                  step={step}
                  queries={resourceQueries}
                  value={resources}
                  onChange={setResources}
                  authenticated={authenticated}
                  onRequireAuth={onRequireAuth}
                  t={t}
                />
              ) : null}
            </fieldset>
          </div>
        </div>
        {error ? (
          <div role="alert" className={styles.error}>
            {error}
          </div>
        ) : null}
        <AgentCreateProgress progress={progress} t={t} />
        <footer className={styles.footer}>
          <Button variant="secondaryGray" disabled={busy} onClick={onClose}>
            {t("cancel")}
          </Button>
          <span />
          <Button variant="secondaryGray" disabled={busy || step === 0} onClick={() => goToStep(step - 1)}>
            {t("agentWizardPrevious")}
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={busy || ((step === 0 || last) && !valid)}
            onClick={() => (last ? void onSave(resources) : goToStep(step + 1))}
          >
            {t(last ? "agentCreateSave" : step > 0 && !steps[step].count ? "agentWizardSkip" : "agentWizardNext")}
          </Button>
        </footer>
      </DialogContent>
    </DialogRoot>
  );
}
