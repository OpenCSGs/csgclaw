import { afterEach, expect, it, vi } from "vitest";
import { fetchFileLocation, fetchWorkspaceDirectory, fileContentURL, openFileLocation } from "@/api/chatFiles";

afterEach(() => vi.unstubAllGlobals());

it("keeps the attachment capability when opening a previewed file", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);
  await openFileLocation(
    "room-1",
    { source: "attachment", attachment_id: "file-1", name: "report.json", label: "report.json" },
    "system",
    "file+capability/=",
  );
  expect(fetchMock).toHaveBeenCalledWith(
    "api/v1/rooms/room-1/attachments/file-1/open?token=file%2Bcapability%2F%3D",
    expect.objectContaining({ method: "POST", body: JSON.stringify({ target: "system" }) }),
  );
});

it("rejects draft browsing on servers that would silently ignore the selected directory", async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname.startsWith("/api/v1/agents/alice/workspace")) {
      if (url.pathname.endsWith("/content")) return new Response("DEFAULT");
      return Response.json({
        absolute_path: "/default",
        entries: [{ name: "same.md", path: "same.md", type: "file" }],
      });
    }
    return new Response("Unsupported endpoint", { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const reference = {
    source: "workspace" as const,
    agent_id: "alice",
    name: "same.md",
    path: "same.md",
    label: "same.md",
  };

  await expect(fetchWorkspaceDirectory("alice", "", undefined, "", "/selected")).rejects.toMatchObject({ status: 404 });
  const preview = await fetch(fileContentURL("", reference, "", "/selected"));
  expect(preview.status).toBe(404);
  await expect(fetchFileLocation("", reference, "", undefined, "/selected")).rejects.toMatchObject({ status: 404 });
  await expect(openFileLocation("", reference, "system", "", "/selected")).rejects.toMatchObject({ status: 404 });
  expect(fetchMock).toHaveBeenCalledTimes(4);
});
