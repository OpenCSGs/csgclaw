import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RemoteMCPList, type RemoteMCPListProps } from "./RemoteMCPList";

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
});
