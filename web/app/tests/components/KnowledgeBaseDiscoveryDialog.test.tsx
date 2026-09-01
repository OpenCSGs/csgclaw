import { fireEvent, render, screen } from "@testing-library/react";
import { KnowledgeBaseDiscoveryDialog } from "@/pages/WorkspacePage/components/WorkspaceSidebar/KnowledgeBaseDiscoveryDialog";

const onLoadMore = vi.fn();
const onSearchChange = vi.fn();

describe("KnowledgeBaseDiscoveryDialog", () => {
  afterEach(() => {
    onLoadMore.mockReset();
    onSearchChange.mockReset();
  });

  it("sends search input to the remote query and loads the next page near the list end", () => {
    render(
      <KnowledgeBaseDiscoveryDialog
        copyBusyID=""
        copyError=""
        hasMore
        items={[
          {
            availability: "available",
            contentID: "content-42",
            id: "42",
            name: "Tourism handbook",
          },
        ]}
        loadError=""
        loading={false}
        loadingMore={false}
        loginRequired={false}
        onAdd={async () => true}
        onLoadMore={onLoadMore}
        onLogin={() => {}}
        onOpenChange={() => {}}
        onRetry={() => {}}
        onSearchChange={onSearchChange}
        open
        search="tourism"
        t={(key) => key}
      />,
    );

    fireEvent.change(screen.getByRole("searchbox", { name: "resourcesKnowledgeBaseSearchPlaceholder" }), {
      target: { value: "investment" },
    });
    expect(onSearchChange).toHaveBeenCalledWith("investment");

    const list = screen.getByText("Tourism handbook").closest("div")?.parentElement;
    expect(list).toBeTruthy();
    Object.defineProperties(list as HTMLDivElement, {
      clientHeight: { configurable: true, value: 100 },
      scrollHeight: { configurable: true, value: 200 },
      scrollTop: { configurable: true, value: 100 },
    });
    fireEvent.scroll(list as HTMLDivElement);
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });
});
