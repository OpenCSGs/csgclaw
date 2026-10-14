import { ComposerAddMenu } from "@/components/business/ConversationPane/ComposerAddMenu";
import type { TranslateFn } from "@/models/conversations";
import type { ResourceSnapshot } from "@/models/conversationResources";
import { ConversationResourcePicker } from "./ConversationResourcePicker";

export type ComposerResourceProps = {
  resourceAgentID?: string;
  onPickResources?: (resources: ResourceSnapshot[]) => void;
};

export function ComposerResourceMenu({
  resourceAgentID,
  onPickResources,
  onManage,
  selected = [],
  disabled,
  t,
  onAddFiles,
}: ComposerResourceProps & {
  selected?: readonly ResourceSnapshot[];
  disabled: boolean;
  t: TranslateFn;
  onAddFiles: () => void;
  onManage?: () => void;
}) {
  return (
    <ComposerAddMenu
      key={`${resourceAgentID}:${disabled}`}
      disabled={disabled}
      t={t}
      onAddFiles={onAddFiles}
      renderResourcePicker={
        resourceAgentID && onPickResources
          ? (kind, close) => (
              <ConversationResourcePicker
                key={`${resourceAgentID}:${kind}`}
                agentID={resourceAgentID}
                kind={kind}
                t={t}
                existing={selected}
                onClose={close}
                onManage={
                  onManage
                    ? () => {
                        close();
                        onManage();
                      }
                    : undefined
                }
                onPick={(refs) => {
                  onPickResources(refs);
                  close();
                }}
              />
            )
          : undefined
      }
    />
  );
}
