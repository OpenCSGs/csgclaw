export type ModelOptionLabelProps = {
  avatar?: string;
  model: string;
  provider?: string;
  showAvatar?: boolean;
};

export function ModelOptionLabel({ avatar, model, provider, showAvatar = true }: ModelOptionLabelProps) {
  const hasAvatar = showAvatar && Boolean(avatar);
  const hasProvider = Boolean(provider);
  return (
    <span className={`model-option-label${hasAvatar ? "" : " no-avatar"}${hasProvider ? "" : " single-line"}`}>
      {hasAvatar ? <img className="model-option-avatar" src={avatar} alt="" aria-hidden="true" /> : null}
      <span className="model-option-copy">
        <span className="model-option-model">{model}</span>
        {hasProvider ? <span className="model-option-provider">{provider}</span> : null}
      </span>
    </span>
  );
}
