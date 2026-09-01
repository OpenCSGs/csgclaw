import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { DocumentPreviewPanel, MAX_TEXT_PREVIEW_BYTES } from "@/components/business/DocumentPreviewPanel";
import type { AttachmentPreviewItem } from "@/models/attachments";

const pdfMock = vi.hoisted(() => ({ pageCount: 1 }));

vi.mock("react-pdf", async () => {
  const React = await import("react");
  return {
    pdfjs: { GlobalWorkerOptions: { workerSrc: "" } },
    Document: ({
      children,
      file,
      onLoadSuccess,
    }: {
      children?: React.ReactNode;
      file?: { data?: Uint8Array };
      onLoadSuccess?: (pdf: { numPages: number }) => void;
    }) => {
      React.useEffect(() => {
        const buffer = file?.data?.buffer;
        if (buffer instanceof ArrayBuffer && buffer.byteLength > 0) {
          structuredClone(buffer, { transfer: [buffer] });
        }
        onLoadSuccess?.({ numPages: pdfMock.pageCount });
      }, [file, onLoadSuccess]);
      return <div data-testid="mock-pdf-document">{children}</div>;
    },
    Page: ({ pageNumber }: { pageNumber: number }) => <div data-page-number={pageNumber} data-testid="mock-pdf-page" />,
  };
});

const mammothMock = vi.hoisted(() => ({ html: "<p>Full-width document body</p>" }));

vi.mock("mammoth", () => ({
  convertToHtml: vi.fn(async () => ({ value: mammothMock.html })),
  images: {
    imgElement: vi.fn((handler) => handler),
  },
}));

const pptxMock = vi.hoisted(() => ({
  counts: [10, 3],
  renderSingleSlide: vi.fn(),
}));

vi.mock("pptx-preview", () => ({
  init: (stage: HTMLElement, options: { mode?: "list" | "slide" }) => {
    const slideCount = pptxMock.counts.shift() ?? 1;
    return {
      destroy: vi.fn(),
      preview: vi.fn(async () => {
        if (options.mode === "list") {
          for (let index = 0; index < slideCount; index += 1) {
            const slide = document.createElement("div");
            slide.className = `pptx-preview-slide-wrapper pptx-preview-slide-wrapper-${index}`;
            stage.append(slide);
          }
        }
      }),
      renderSingleSlide: pptxMock.renderSingleSlide,
      slideCount,
    };
  },
}));

const t = (key: string, params?: Record<string, string | number>) => {
  const labels: Record<string, string> = {
    attachmentPreview: "Attachment preview",
    attachmentPreviewFailed: "Preview failed",
    attachmentPreviewFit: "Fit",
    attachmentPreviewFullscreen: "Fullscreen",
    attachmentPreviewExitFullscreen: "Exit fullscreen",
    attachmentPreviewLoading: "Loading preview",
    attachmentPreviewPageCount: `Page ${params?.page ?? 0} of ${params?.count ?? 0}`,
    attachmentPreviewSlideCount: `Slide ${params?.page ?? 0} of ${params?.count ?? 0}`,
    attachmentPreviewTruncated: "Only the first 256 KiB is shown",
    attachmentPreviewResetZoom: "Reset zoom",
    attachmentPreviewResize: "Resize preview",
    attachmentPreviewUnavailable: "Preview unavailable",
    attachmentPreviewZoomIn: "Zoom in",
    attachmentPreviewZoomOut: "Zoom out",
    attachmentsScrollNext: "Next file",
    attachmentsScrollPrevious: "Previous file",
    close: "Close",
    downloadAttachment: "Download",
  };
  return labels[key] ?? `${key}${params ? JSON.stringify(params) : ""}`;
};

describe("DocumentPreviewPanel", () => {
  beforeEach(() => {
    pdfMock.pageCount = 1;
    mammothMock.html = "<p>Full-width document body</p>";
    pptxMock.counts = [10, 3];
    pptxMock.renderSingleSlide.mockClear();
  });
  const items: AttachmentPreviewItem[] = [
    {
      id: "markdown",
      name: "report.md",
      mediaType: "text/markdown",
      sizeBytes: 18,
      previewURL: "api/v1/attachments/markdown?token=one",
      downloadURL: "api/v1/attachments/markdown?token=one",
    },
    {
      id: "legacy",
      name: "slides.ppt",
      mediaType: "application/vnd.ms-powerpoint",
      sizeBytes: 20,
      previewURL: "api/v1/attachments/legacy?token=two",
      downloadURL: "api/v1/attachments/legacy?token=two",
    },
  ];

  it("loads, renders, navigates, zooms, downloads, and restores focus", async () => {
    const user = userEvent.setup();
    const anchor = document.createElement("button");
    document.body.append(anchor);
    const onClose = vi.fn();
    const onIndexChange = vi.fn();
    const fetchMock = vi.fn<typeof fetch>(async (url) => {
      const body = String(url).includes("markdown")
        ? "# Generated report"
        : new Uint8Array([0xd0, 0xcf, 0x11, 0xe0, 0x00, 0xff]);
      return new Response(body, { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { rerender, unmount } = render(
      <DocumentPreviewPanel
        anchor={anchor}
        index={0}
        items={items}
        t={t}
        onClose={onClose}
        onIndexChange={onIndexChange}
      />,
    );
    expect(await screen.findByRole("heading", { name: "Generated report" })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("api/v1/attachments/markdown?token=one", {
      credentials: "same-origin",
      signal: expect.any(AbortSignal),
    });
    await user.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(screen.getByText("110%")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Next file" }));
    expect(onIndexChange).toHaveBeenCalledWith(1);
    expect(screen.getByRole("link", { name: "Download" })).toHaveAttribute(
      "href",
      "api/v1/attachments/markdown?token=one",
    );
    expect(screen.getByRole("button", { name: "Close" })).toHaveClass("document-preview-close-button");

    rerender(
      <DocumentPreviewPanel
        anchor={anchor}
        index={1}
        items={items}
        t={t}
        onClose={onClose}
        onIndexChange={onIndexChange}
      />,
    );
    expect(await screen.findByText("Preview unavailable")).toBeInTheDocument();
    unmount();
    await waitFor(() => expect(document.activeElement).toBe(anchor));
    anchor.remove();
    vi.unstubAllGlobals();
  });

  it("previews generic attachments when their bytes are text", async () => {
    const { container } = render(
      <DocumentPreviewPanel
        index={0}
        items={[
          {
            file: new File(['title = "CSGClaw"\n[server]\nport = 18080'], "settings.toml", {
              type: "application/octet-stream",
            }),
            id: "toml",
            mediaType: "application/octet-stream",
            name: "settings.toml",
            sizeBytes: 43,
          },
        ]}
        t={t}
        onClose={() => {}}
        onIndexChange={() => {}}
      />,
    );

    await waitFor(() =>
      expect(container.querySelector(".document-preview-shiki")).toHaveTextContent('title = "CSGClaw"'),
    );
    expect(container.querySelector(".document-preview-shiki")).toHaveAttribute("data-language", "toml");
    expect(screen.queryByText("Preview unavailable")).not.toBeInTheDocument();
  });

  it("syntax-highlights Go source with dual light and dark theme tokens", async () => {
    const { container } = render(
      <DocumentPreviewPanel
        index={0}
        items={[
          {
            file: new File(['package main\n\nimport "fmt"\n\nfunc main() { fmt.Println("hello") }\n'], "main.go", {
              type: "text/plain",
            }),
            id: "go",
            mediaType: "text/plain",
            name: "main.go",
            sizeBytes: 65,
          },
        ]}
        t={t}
        onClose={() => {}}
        onIndexChange={() => {}}
      />,
    );

    await waitFor(() =>
      expect(container.querySelector('.document-preview-shiki[data-language="go"]')).toHaveTextContent("package main"),
    );
    const tokens = container.querySelectorAll<HTMLElement>(".document-preview-shiki .shiki span[style]");
    expect(tokens.length).toBeGreaterThan(3);
    expect(Array.from(tokens).some((token) => token.getAttribute("style")?.includes("--shiki-dark"))).toBe(true);
  });

  it("clamps a persisted oversized width to the preview host", async () => {
    window.localStorage.setItem("csgclaw.im.documentPreviewPanelWidth", "1600");
    const rectSpy = vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
      this: HTMLElement,
    ) {
      return this.dataset.previewHost === "true" ? new DOMRect(0, 0, 1280, 800) : new DOMRect();
    });
    try {
      const { container } = render(
        <div data-preview-host="true">
          <DocumentPreviewPanel
            index={0}
            items={[
              {
                file: new File(["plain text"], "notes.txt", { type: "text/plain" }),
                id: "text",
                mediaType: "text/plain",
                name: "notes.txt",
                sizeBytes: 10,
              },
            ]}
            t={t}
            onClose={() => {}}
            onIndexChange={() => {}}
          />
        </div>,
      );

      await waitFor(() =>
        expect(container.querySelector(".document-preview-panel")).toHaveStyle("--document-preview-panel-width: 920px"),
      );
      expect(screen.getByRole("button", { name: "Close" })).toBeVisible();
    } finally {
      rectSpy.mockRestore();
      window.localStorage.removeItem("csgclaw.im.documentPreviewPanelWidth");
    }
  });

  it("switches from a worker-transferred PDF to TOML without detaching shared preview data", async () => {
    const items: AttachmentPreviewItem[] = [
      {
        file: new File([new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x00])], "sample.pdf", {
          type: "application/pdf",
        }),
        id: "pdf",
        mediaType: "application/pdf",
        name: "sample.pdf",
        sizeBytes: 5,
      },
      {
        file: new File(['severity = "warning"\n'], "revive.toml", { type: "application/octet-stream" }),
        id: "toml-after-pdf",
        mediaType: "application/octet-stream",
        name: "revive.toml",
        sizeBytes: 21,
      },
    ];
    const { container, rerender } = render(
      <DocumentPreviewPanel index={0} items={items} t={t} onClose={() => {}} onIndexChange={() => {}} />,
    );
    expect(await screen.findByTestId("mock-pdf-page")).toBeInTheDocument();

    rerender(<DocumentPreviewPanel index={1} items={items} t={t} onClose={() => {}} onIndexChange={() => {}} />);
    await waitFor(() =>
      expect(container.querySelector('.document-preview-shiki[data-language="toml"]')).toHaveTextContent(
        'severity = "warning"',
      ),
    );
    expect(screen.queryByText(/Unexpected Application Error/)).not.toBeInTheDocument();
  });

  it("resets PowerPoint navigation when switching to a shorter deck", async () => {
    const items: AttachmentPreviewItem[] = [
      {
        file: new File(["first deck"], "first.pptx", {
          type: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
        }),
        id: "pptx-first",
        mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
        name: "first.pptx",
        sizeBytes: 10,
      },
      {
        file: new File(["short deck"], "short.pptx", {
          type: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
        }),
        id: "pptx-short",
        mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
        name: "short.pptx",
        sizeBytes: 10,
      },
    ];
    const { container, rerender } = render(
      <DocumentPreviewPanel index={0} items={items} t={t} onClose={() => {}} onIndexChange={() => {}} />,
    );
    expect(await screen.findByText("Slide 1 of 10")).toBeInTheDocument();
    expect(container.querySelectorAll(".pptx-preview-slide-wrapper")).toHaveLength(10);
    const nextSlide = container.querySelector<HTMLButtonElement>(".document-preview-page-controls button:last-child");
    expect(nextSlide).not.toBeNull();
    for (let index = 1; index < 10; index += 1) {
      await userEvent.click(nextSlide!);
    }
    expect(screen.getByText("Slide 10 of 10")).toBeInTheDocument();

    rerender(<DocumentPreviewPanel index={1} items={items} t={t} onClose={() => {}} onIndexChange={() => {}} />);
    expect(await screen.findByText("Slide 1 of 3")).toBeInTheDocument();
    expect(container.querySelectorAll(".pptx-preview-slide-wrapper")).toHaveLength(3);
    expect(screen.queryByText("Slide 10 of 3")).not.toBeInTheDocument();
  });

  it("bounds large text previews and keeps the complete file downloadable", async () => {
    const fullText = `${"x".repeat(MAX_TEXT_PREVIEW_BYTES + 32)}TAIL`;
    const { container } = render(
      <DocumentPreviewPanel
        index={0}
        items={[
          {
            file: new File([fullText], "large.txt", { type: "text/plain" }),
            id: "large-text",
            mediaType: "text/plain",
            name: "large.txt",
            sizeBytes: fullText.length,
          },
        ]}
        t={t}
        onClose={() => {}}
        onIndexChange={() => {}}
      />,
    );

    expect(await screen.findByRole("status")).toHaveTextContent("Only the first 256 KiB is shown");
    const preview = container.querySelector(".document-preview-text");
    expect(preview?.textContent).toHaveLength(MAX_TEXT_PREVIEW_BYTES);
    expect(preview).not.toHaveTextContent("TAIL");
    expect(screen.getByRole("link", { name: "Download" })).toHaveAttribute("download", "large.txt");
  });

  it("toggles the same control between entering and exiting fullscreen", async () => {
    const user = userEvent.setup();
    let fullscreenElement: Element | null = null;
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => fullscreenElement,
    });
    const exitFullscreen = vi.fn(async () => {
      fullscreenElement = null;
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    Object.defineProperty(document, "exitFullscreen", { configurable: true, value: exitFullscreen });

    try {
      const { container } = render(
        <DocumentPreviewPanel
          index={0}
          items={[
            {
              file: new File(["preview"], "preview.txt", { type: "text/plain" }),
              id: "fullscreen-text",
              mediaType: "text/plain",
              name: "preview.txt",
              sizeBytes: 7,
            },
          ]}
          t={t}
          onClose={() => {}}
          onIndexChange={() => {}}
        />,
      );
      const panel = container.querySelector<HTMLElement>(".document-preview-panel");
      expect(panel).not.toBeNull();
      const requestFullscreen = vi.fn(async () => {
        fullscreenElement = panel;
        document.dispatchEvent(new Event("fullscreenchange"));
      });
      Object.defineProperty(panel!, "requestFullscreen", { configurable: true, value: requestFullscreen });

      await user.click(screen.getByRole("button", { name: "Fullscreen" }));
      expect(requestFullscreen).toHaveBeenCalledTimes(1);
      await user.click(screen.getByRole("button", { name: "Exit fullscreen" }));
      expect(exitFullscreen).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("button", { name: "Fullscreen" })).toBeInTheDocument();
    } finally {
      Reflect.deleteProperty(document, "fullscreenElement");
      Reflect.deleteProperty(document, "exitFullscreen");
    }
  });

  it("renders PDF pages as a vertically scrollable continuous document", async () => {
    const user = userEvent.setup();
    pdfMock.pageCount = 3;
    const { container } = render(
      <DocumentPreviewPanel
        index={0}
        items={[
          {
            file: new File([new Uint8Array([0x25, 0x50, 0x44, 0x46])], "report.pdf", {
              type: "application/pdf",
            }),
            id: "continuous-pdf",
            mediaType: "application/pdf",
            name: "report.pdf",
            sizeBytes: 4,
          },
        ]}
        t={t}
        onClose={() => {}}
        onIndexChange={() => {}}
      />,
    );

    expect(await screen.findAllByTestId("mock-pdf-page")).toHaveLength(2);
    expect(container.querySelectorAll(".document-preview-pdf-page")).toHaveLength(3);
    const secondPage = container.querySelector<HTMLElement>('.document-preview-pdf-page[data-page-number="2"]');
    expect(secondPage).not.toBeNull();
    const scrollIntoView = vi.fn();
    Object.defineProperty(secondPage!, "scrollIntoView", { configurable: true, value: scrollIntoView });
    await user.click(screen.getByRole("button", { name: "Next file" }));
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" });
    expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
    expect(await screen.findAllByTestId("mock-pdf-page")).toHaveLength(3);
  });

  it("uses the full DOCX preview width when no outline is available", async () => {
    const { container } = render(
      <DocumentPreviewPanel
        index={0}
        items={[
          {
            file: new File(["docx"], "report.docx", {
              type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            }),
            id: "docx-without-outline",
            mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            name: "report.docx",
            sizeBytes: 4,
          },
        ]}
        t={t}
        onClose={() => {}}
        onIndexChange={() => {}}
      />,
    );

    expect(await screen.findByText("Full-width document body")).toBeInTheDocument();
    expect(container.querySelector(".document-preview-docx-shell")).toHaveClass("without-outline");
    expect(container.querySelector(".document-preview-docx-outline")).not.toBeInTheDocument();
  });
});
