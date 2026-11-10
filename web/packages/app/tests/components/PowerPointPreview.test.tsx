import { render, screen, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import PowerPointPreview from "@/components/business/DocumentPreviewPanel/PowerPointPreview";

const renderer = vi.hoisted(() => ({
  preview: vi.fn<() => Promise<unknown>>(),
  destroy: vi.fn(),
  slideCount: 0,
}));
vi.mock("pptx-preview", () => ({ init: () => renderer }));
vi.mock("@/components/business/DocumentPreviewPanel/normalizePowerPoint", () => ({
  normalizePowerPointPreview: async (data: ArrayBuffer) => data,
}));
const t = (key: string) => key;

beforeEach(() => {
  renderer.slideCount = 0;
  renderer.preview.mockReset();
  renderer.destroy.mockReset();
});

it("shows loading while the renderer is pending and clears it when slides are ready", async () => {
  let complete!: () => void;
  renderer.preview.mockImplementation(() => new Promise<void>((resolve) => (complete = resolve)));
  render(<PowerPointPreview data={new ArrayBuffer(1)} scale={1} t={t} />);
  expect(screen.getByRole("status")).toHaveTextContent("attachmentPreviewLoading");
  await waitFor(() => expect(renderer.preview).toHaveBeenCalled());
  renderer.slideCount = 20;
  complete();
  expect(await screen.findByText("attachmentPreviewSlideCount")).toBeInTheDocument();
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it("reports failure when the renderer silently returns no slides", async () => {
  renderer.preview.mockResolvedValue(undefined);
  render(<PowerPointPreview data={new ArrayBuffer(1)} scale={1} t={t} />);
  expect(await screen.findByText("attachmentPreviewFailed")).toBeInTheDocument();
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});
