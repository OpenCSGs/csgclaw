import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FileTypeIcon } from "@/components/business/FileTypeIcon";

describe("FileTypeIcon", () => {
  it("uses the Figma folder icon without a document page or generic mark", () => {
    const { container } = render(<FileTypeIcon isFolder />);

    expect(container.firstElementChild).toHaveAttribute("aria-hidden", "true");
    expect(container.querySelector("img")).not.toBeInTheDocument();
  });

  it("uses a format-specific label instead of the generic document mark for Markdown", () => {
    const { container } = render(<FileTypeIcon name="README.md" />);

    expect(screen.getByText("MD")).toBeInTheDocument();
    expect(container.querySelectorAll("img")).toHaveLength(1);
  });

  it("uses the matching SVG automatically when an asset exists for the extension", () => {
    render(<FileTypeIcon name="snippet.js" />);

    expect(screen.getByText("JS")).toBeInTheDocument();
    expect(document.querySelectorAll("img")).toHaveLength(1);
  });

  it("keeps generic marks for formats without a dedicated icon", () => {
    const { container } = render(<FileTypeIcon name="notes.odt" />);

    expect(screen.queryByText("ODT")).not.toBeInTheDocument();
    expect(container.querySelectorAll("img")).toHaveLength(2);
  });
});
