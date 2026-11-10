import { resourceKey, resourceLabel, type ResourceSnapshot } from "@/models/agentResources";

type ResourceTagProps = {
  resource: ResourceSnapshot;
  removal?: { label: string; disabled?: boolean; onRemove: () => void };
};

export function ResourceTag({ resource, removal }: ResourceTagProps) {
  return (
    <span className="resource-tag" data-resource-type={resource.type} title={resourceKey(resource)}>
      <span className="resource-tag-label">{resourceLabel(resource)}</span>
      {removal ? (
        <button
          type="button"
          className="resource-tag-remove"
          aria-label={`${removal.label} ${resourceLabel(resource)}`}
          disabled={removal.disabled}
          onClick={removal.onRemove}
        />
      ) : null}
    </span>
  );
}
