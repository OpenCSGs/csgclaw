import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ConversationMessageActions } from "@/components/business/ConversationPane/ConversationMessageActions";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) =>
  ({
    copiedToClipboard: "Copied",
    copyToClipboard: "Copy",
    copyImage: "Copy image",
    copyingImage: "Copying image",
    copyImageFailed: "Could not copy image",
    copyTextFailed: "Could not copy text",
    replyInThread: "Reply in thread",
  })[key] || key;

describe("ConversationMessageActions", () => {
  it("copies the complete message source with visible mention text", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    render(<ConversationMessageActions content={'First line\nSecond line for <at user_id="u-1">Alice</at>'} t={t} />);

    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("First line\nSecond line for @Alice"));
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("copies canonical slash commands in their visible input format", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    render(
      <ConversationMessageActions
        content={
          '<slash-command name="use-skill" arg="reviewer"></slash-command> review with <at user_id="u-1">Alice</at>'
        }
        t={t}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith("/reviewer review with @Alice"));
  });

  it("opens the thread from the message action row", () => {
    const onOpenThread = vi.fn();
    render(<ConversationMessageActions content="Message" onOpenThread={onOpenThread} t={t} />);

    fireEvent.click(screen.getByRole("button", { name: "Reply in thread" }));

    expect(onOpenThread).toHaveBeenCalledTimes(1);
  });
});

it("keeps the task reference in the same action strip as copy and reply", () => {
  render(
    <ConversationMessageActions
      leading={<button>View task #11</button>}
      content="Done"
      onOpenThread={() => {}}
      t={t}
    />,
  );
  const strip = screen.getByRole("button", { name: "Copy" }).parentElement;
  expect(strip).toContainElement(screen.getByRole("button", { name: "View task #11" }));
  expect(strip).toContainElement(screen.getByRole("button", { name: "Reply in thread" }));
});

describe("generated image clipboard", () => {
  const imageAttachment = {
    id: "image-1",
    name: "image.png",
    media_type: "image/png",
    kind: "image",
    download_url: "api/v1/attachments/image-1",
    created_at: "2026-09-15T00:00:00Z",
    sha256: "image-sha",
    size_bytes: 100,
  };
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  function clipboard() {
    let copiedBlob: Blob | undefined;
    class ImageClipboardItem {
      constructor(public data: Record<string, Promise<Blob>>) {}
    }
    vi.stubGlobal("ClipboardItem", ImageClipboardItem);
    const write = vi.fn(async (items: ImageClipboardItem[]) => {
      copiedBlob = await items[0].data["image/png"];
    });
    const writeText = vi.fn();
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { write, writeText } });
    return { write, writeText, blob: () => copiedBlob };
  }

  it("copies PNG image data instead of its prompt or URL", async () => {
    const clip = clipboard();
    const png = new Blob(["png pixels"], { type: "image/png" });
    const fetchImage = vi.fn().mockResolvedValue({ ok: true, blob: async () => png });
    vi.stubGlobal("fetch", fetchImage);
    render(
      <ConversationMessageActions
        content="expanded prompt"
        image={{ ...imageAttachment, download_url: "/api/v1/attachments/image-1" }}
        t={t}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy image" }));
    await screen.findByRole("button", { name: "Copied" });
    expect(clip.blob()).toBe(png);
    expect(clip.writeText).not.toHaveBeenCalled();
    expect(fetchImage).toHaveBeenCalledWith(
      "api/v1/attachments/image-1",
      expect.objectContaining({ credentials: "same-origin" }),
    );
  });

  it("converts JPEG to clipboard PNG and releases the decoded bitmap", async () => {
    const clip = clipboard();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, blob: async () => new Blob(["jpeg"], { type: "image/jpeg" }) }),
    );
    const close = vi.fn();
    const drawImage = vi.fn();
    vi.stubGlobal("createImageBitmap", vi.fn().mockResolvedValue({ width: 320, height: 180, close }));
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage,
    } as unknown as CanvasRenderingContext2D);
    const png = new Blob(["converted png"], { type: "image/png" });
    vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((callback) => callback(png));
    render(
      <ConversationMessageActions
        image={{ ...imageAttachment, download_url: "api/v1/attachments/image-jpeg", media_type: "image/jpeg" }}
        t={t}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy image" }));
    await screen.findByRole("button", { name: "Copied" });
    expect(clip.blob()).toBe(png);
    expect(close).toHaveBeenCalledOnce();
    expect(drawImage).toHaveBeenCalledOnce();
  });

  it("shows failure without silently copying the prompt", async () => {
    const clip = clipboard();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false }));
    render(
      <ConversationMessageActions
        content="do not copy this prompt"
        image={{ ...imageAttachment, download_url: "api/v1/attachments/expired" }}
        t={t}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy image" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not copy image");
    expect(clip.writeText).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Copied" })).toBeNull();
  });
});
