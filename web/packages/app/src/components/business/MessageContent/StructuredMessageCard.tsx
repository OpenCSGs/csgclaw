import type { StructuredMessagePayload } from "./types";

export type StructuredMessageCardProps = {
  data: StructuredMessagePayload;
};

export function StructuredMessageCard({ data }: StructuredMessageCardProps) {
  return (
    <div className="structured-message">
      <StructuredMessageTitleBlock data={data} />
      {data.summary ? <div className="structured-message-summary">{data.summary}</div> : null}
      {data.link && isSafeHttpURL(data.link) ? (
        <div className="structured-message-link">
          <a href={data.link} target="_blank" rel="noopener noreferrer">
            Open link
          </a>
        </div>
      ) : null}
      {data.meta?.length ? (
        <div className="structured-message-meta">
          {data.meta.map((row, index) => (
            <div key={`${row.label}-${index}`} className="structured-message-meta-row">
              <span className="structured-message-meta-label">{row.label}</span>
              <span className="structured-message-meta-value">{row.value}</span>
            </div>
          ))}
        </div>
      ) : null}
      {data.code ? (
        <details className="structured-message-details">
          <summary>{data.codeSummary}</summary>
          <pre className="structured-message-code">
            <code>{data.code}</code>
          </pre>
        </details>
      ) : null}
      {data.payload ? (
        <details className="structured-message-details">
          <summary>{data.payloadSummary}</summary>
          <pre className="structured-message-json">
            <code>{data.payload}</code>
          </pre>
        </details>
      ) : null}
    </div>
  );
}

export function StructuredMessageTitleBlock({ data }: StructuredMessageCardProps) {
  return (
    <div className="structured-message-header">
      <div>
        <div className="structured-message-title">{data.title}</div>
        {data.subtitle ? <div className="structured-message-subtitle">{data.subtitle}</div> : null}
      </div>
      {data.badge ? <span className="structured-message-badge">{data.badge}</span> : null}
    </div>
  );
}

function isSafeHttpURL(url: string) {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}
