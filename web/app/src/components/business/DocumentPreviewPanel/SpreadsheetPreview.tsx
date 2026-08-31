import { useEffect, useState } from "react";
import { copyPreviewBuffer } from "./previewBuffer";

type Sheet = { name: string; rows: unknown[][] };

export default function SpreadsheetPreview({
  data,
  scale,
  t,
}: {
  data: ArrayBuffer;
  scale: number;
  t: (key: string) => string;
}) {
  const [sheets, setSheets] = useState<Sheet[]>([]);
  const [active, setActive] = useState(0);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setSheets([]);
    setActive(0);
    setError(false);
    void import("xlsx")
      .then((xlsx) => {
        const workbook = xlsx.read(copyPreviewBuffer(data), { type: "array" });
        return workbook.SheetNames.map((name) => ({
          name,
          rows: xlsx.utils.sheet_to_json<unknown[]>(workbook.Sheets[name], { header: 1, raw: false }),
        }));
      })
      .then((nextSheets) => {
        if (!cancelled) {
          setSheets(nextSheets);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [data]);

  if (error) {
    return <div className="document-preview-status is-error">{t("attachmentPreviewFailed")}</div>;
  }
  if (sheets.length === 0) {
    return <div className="document-preview-status">{t("attachmentPreviewLoading")}</div>;
  }
  const sheet = sheets[active] ?? sheets[0];
  return (
    <div className="document-preview-spreadsheet" style={{ fontSize: `${scale}em` }}>
      <div className="document-preview-sheet-tabs" role="tablist" aria-label={t("attachmentPreviewSheets")}>
        {sheets.map((candidate, index) => (
          <button
            key={candidate.name}
            type="button"
            role="tab"
            aria-selected={index === active}
            className={index === active ? "active" : ""}
            onClick={() => setActive(index)}
          >
            {candidate.name}
          </button>
        ))}
      </div>
      <div className="document-preview-table-wrap">
        <table>
          <tbody>
            {sheet.rows.map((row, rowIndex) => (
              <tr key={rowIndex}>
                <th scope="row">{rowIndex + 1}</th>
                {row.map((cell, cellIndex) => (
                  <td key={cellIndex}>{cell == null ? "" : String(cell)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
