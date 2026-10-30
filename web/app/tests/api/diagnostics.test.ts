import JSZip from "jszip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchDiagnosticArchive, fetchDiagnosticBundle } from "@/api/diagnostics";

afterEach(() => vi.unstubAllGlobals());

async function zipResponse(metadata: unknown, name = "csgclaw-turn-abc-diagnostics.json") {
  const zip = new JSZip();
  // Intentionally not JSON: a successful download must not parse or inflate
  // the full entry to discover its turn ID and collection status.
  zip.file(name, "large full JSON is opaque to the download client");
  return new Response(
    await zip.generateAsync({ type: "arraybuffer", compression: "DEFLATE", comment: JSON.stringify(metadata) }),
    {
      headers: { "Content-Type": "application/zip" },
    },
  );
}

describe("diagnostic ZIP download", () => {
  it("downloads binary ZIP and reads only its small status metadata", async () => {
    const response = await zipResponse({ schema_version: 1, turn_id: "turn-csgclaw-abc", complete: false });
    const text = vi.spyOn(response, "text");
    const fetch = vi.fn().mockResolvedValue(response);
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;
    const result = await fetchDiagnosticArchive("room/one", "diagnostic/id", signal);
    expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(/^api\/v1\/rooms\/room%2Fone\/diagnostics\/diagnostic%2Fid\/bundle\?/),
      expect.objectContaining({ signal, headers: { Accept: "application/zip" }, cache: "no-store" }),
    );
    expect(fetch.mock.calls[0][0]).toContain("download=1");
    expect(result).toMatchObject({
      filename: "csgclaw-turn-abc-diagnostics.zip",
      turnId: "turn-csgclaw-abc",
      complete: false,
    });
    expect(result.blob.type).toBe("application/zip");
    expect(result.blob.size).toBeGreaterThan(0);
    expect(text).not.toHaveBeenCalled();
  });

  it("preserves structured HTTP errors instead of saving them as ZIP", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: { code: "diagnostic_unavailable", message: "Expired" } }), {
          status: 404,
        }),
      ),
    );
    await expect(fetchDiagnosticArchive("room", "id")).rejects.toMatchObject({
      status: 404,
      code: "diagnostic_unavailable",
    });
  });

  it("rejects truncated ZIP data", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("PK incomplete ZIP")));
    await expect(fetchDiagnosticArchive("room", "id")).rejects.toThrow();
  });

  it.each([
    null,
    { schema_version: 1, turn_id: "turn-csgclaw-abc", complete: "false" },
    { schema_version: 2, turn_id: "turn-csgclaw-abc", complete: false },
  ])("rejects invalid archive status metadata: %j", async (metadata) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(await zipResponse(metadata)));
    await expect(fetchDiagnosticArchive("room", "id")).rejects.toThrow("Invalid diagnostic archive metadata");
  });

  it("rejects an archive that is missing the expected JSON entry", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          await zipResponse({ schema_version: 1, turn_id: "turn-csgclaw-abc", complete: true }, "wrong.json"),
        ),
    );
    await expect(fetchDiagnosticArchive("room", "id")).rejects.toThrow("Diagnostic archive entry missing");
  });

  it("does not finish a cancelled export", async () => {
    const response = await zipResponse({ schema_version: 1, turn_id: "turn-csgclaw-abc", complete: true });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
    const controller = new AbortController();
    controller.abort();
    await expect(fetchDiagnosticArchive("room", "id", controller.signal)).rejects.toMatchObject({ name: "AbortError" });
  });
});

it("keeps copy diagnostics as the original complete JSON text", async () => {
  const text =
    '{\n  "schema_version": 1,\n  "turn_id": "turn-csgclaw-abc",\n  "record_id": 9007199254740993,\n  "collection": { "complete": false }\n}\n';
  const fetch = vi.fn().mockResolvedValue(new Response(text));
  vi.stubGlobal("fetch", fetch);
  const result = await fetchDiagnosticBundle("room", "id");
  expect(result).toEqual({ text, turnId: "turn-csgclaw-abc", complete: false });
  expect(fetch.mock.calls[0][0]).not.toContain("download=");
});
