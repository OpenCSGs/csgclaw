import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RemoteMCPList, type RemoteMCPListProps } from "./RemoteMCPList";
import styles from "./RemoteMCPList.module.css";

const baseProps: RemoteMCPListProps = {
  error: "MCP构建失败或权限不足",
  hasMore: false,
  installedServers: [],
  installBusy: "",
  items: [],
  loading: false,
  loadingMore: false,
  search: "",
  t: (key) => key,
};

describe("RemoteMCPList", () => {
  it("runs refresh from the error state and disables repeated clicks while loading", () => {
    const onRefresh = vi.fn();
    const { rerender } = render(<RemoteMCPList {...baseProps} onRefresh={onRefresh} />);

    fireEvent.click(screen.getByRole("button", { name: "resourcesMCPRemoteServersRefresh" }));
    expect(onRefresh).toHaveBeenCalledOnce();

    rerender(<RemoteMCPList {...baseProps} loading onRefresh={onRefresh} />);
    expect(screen.getByRole("button", { name: "resourcesMCPRemoteServersRefresh" })).toBeDisabled();
  });

  it("labels recommended MCP servers without changing their order", () => {
    render(
      <RemoteMCPList
        {...baseProps}
        error=""
        items={[
          { id: "z", name: "Zulu", description: "First server", url: "", recommended: false },
          { id: "a", name: "Alpha", description: "Second server", url: "", recommended: true },
        ]}
      />,
    );
    const first = screen.getByText("Zulu");
    const second = screen.getByText("Alpha");
    const badge = screen.getByText("resourceRecommended");
    expect(first.compareDocumentPosition(second)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(second.parentElement).toContainElement(badge);
  });

  it("renders errors with alert semantics and danger styling", () => {
    render(<RemoteMCPList {...baseProps} />);

    expect(screen.getByRole("alert")).toHaveClass(styles.errorState);
  });
});
