import { fireEvent, render, screen } from "@testing-library/react";
import { SlashPicker } from "@/components/business/ConversationPane/SlashPicker";
import { handleSlashPickerNavigation } from "@/components/business/ConversationPane/slashPickerNavigation";
import type { KeyboardEvent } from "react";

it("limits rendered options and shows the visible portion after scrolling", () => {
  const candidates = Array.from({ length: 200 }, (_, index) => ({ name: `skill-${index}`, type: "skill" as const }));
  const { rerender } = render(<SlashPicker candidates={candidates} t={(key) => key} onSelect={vi.fn()} />);
  expect(screen.getAllByRole("option").length).toBeLessThanOrEqual(24);
  fireEvent.scroll(screen.getByRole("listbox"), { target: { scrollTop: 2200 } });
  expect(screen.getByRole("option", { name: /skill-50 / })).toBeInTheDocument();
  Object.defineProperty(screen.getByRole("listbox"), "clientHeight", { value: 352 });
  rerender(<SlashPicker candidates={candidates.slice(0, 3)} t={(key) => key} onSelect={vi.fn()} />);
  expect(screen.getByRole("option", { name: /skill-0 / })).toBeInTheDocument();
  expect(screen.getAllByRole("option").length).toBeLessThanOrEqual(24);
});

it("continues keyboard selection into the next batch and preserves selection on failure", async () => {
  const onIndexChange = vi.fn();
  const loadMore = vi.fn().mockResolvedValue(true);
  const input = {
    event: { key: "ArrowDown", preventDefault: vi.fn() } as unknown as KeyboardEvent<HTMLElement>,
    candidates: [{ name: "alpha", type: "skill" as const }],
    activeIndex: 0,
    pickerOpen: true,
    onIndexChange,
    onApply: vi.fn(),
    onDismiss: vi.fn(),
    continuation: { hasMore: true, loading: false, failed: false, loadMore, retry: vi.fn() },
  };
  expect(handleSlashPickerNavigation(input)).toBe(true);
  await Promise.resolve();
  expect(onIndexChange).toHaveBeenCalledWith(1);
  onIndexChange.mockClear();
  loadMore.mockResolvedValue(false);
  handleSlashPickerNavigation(input);
  await Promise.resolve();
  expect(onIndexChange).not.toHaveBeenCalled();
});
