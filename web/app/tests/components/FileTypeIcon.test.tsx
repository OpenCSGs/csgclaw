import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FileTypeIcon } from "@/components/business/ChatSidePanel/FileTypeIcon";

describe("FileTypeIcon", () => {
  it("uses the Figma folder icon without a document page or generic mark", () => {
    render(<FileTypeIcon isFolder />);

    expect(document.querySelector(".chat-file-type-icon.is-folder > img")).toHaveAttribute("src");
    expect(document.querySelector(".chat-file-type-icon.is-folder > img")).toHaveAttribute("width", "24");
    expect(document.querySelector(".chat-file-type-icon.is-folder > img")).toHaveAttribute("height", "24");
    expect(document.querySelector(".chat-file-type-mark")).not.toBeInTheDocument();
  });

  it("uses a format-specific label instead of the generic document mark for Markdown", () => {
    render(<FileTypeIcon name="README.md" />);

    expect(screen.getByText("MD")).toBeInTheDocument();
    expect(document.querySelector(".chat-file-type-icon > img")).toHaveAttribute("src");
    expect(document.querySelector(".chat-file-type-mark")).not.toBeInTheDocument();
  });

  it("uses the matching SVG automatically when an asset exists for the extension", () => {
    render(<FileTypeIcon name="snippet.js" />);

    expect(screen.getByText("JS")).toBeInTheDocument();
    expect(document.querySelector(".chat-file-type-mark")).not.toBeInTheDocument();
  });

  it("keeps generic marks for formats without a dedicated icon", () => {
    render(<FileTypeIcon name="notes.odt" />);

    expect(screen.queryByText("ODT")).not.toBeInTheDocument();
    expect(document.querySelector(".chat-file-type-mark")).toBeInTheDocument();
  });
});
