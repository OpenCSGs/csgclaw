import { afterEach, expect, it, vi } from "vitest";
import { openFileLocation } from "@/api/chatFiles";

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
