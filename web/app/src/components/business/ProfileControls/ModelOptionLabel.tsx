export type ModelOptionLabelProps = {
  avatar?: string;
  badge?: string;
  model: string;
  provider?: string;
  showAvatar?: boolean;
  title?: string;
};

export function ModelOptionLabel({ avatar, badge, model, provider, showAvatar = true, title }: ModelOptionLabelProps) {
  const hasAvatar = showAvatar && Boolean(avatar);
  const hasProvider = Boolean(provider);
  const resolvedTitle = title || [model, provider].filter(Boolean).join(" · ");
  return (
    <span
      className={`model-option-label${hasAvatar ? "" : " no-avatar"}${hasProvider ? "" : " single-line"}`}
      title={resolvedTitle}
    >
      {hasAvatar ? <img className="model-option-avatar" src={avatar} alt="" aria-hidden="true" /> : null}
      <span className="model-option-copy">
        <span className="model-option-model">
          <span className="model-option-model-name">{model}</span>
          {badge ? <span className="model-option-badge">{badge}</span> : null}
        </span>
        {hasProvider ? <span className="model-option-provider">{provider}</span> : null}
      </span>
    </span>
  );
}
