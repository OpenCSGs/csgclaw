import { renderHook } from "@testing-library/react";
import { useSlashSelection } from "@/hooks/workspace/useSlashSelection";
import type { SlashPickerCandidate } from "@/models/slashCommands";

it("keeps the default selection at the top when the first skill batch arrives", () => {
  const command: SlashPickerCandidate = { name: "创建智能体", type: "command" };
  const room: SlashPickerCandidate = { name: "创建房间", type: "command" };
  const skill: SlashPickerCandidate = { name: "alpha", type: "skill" };
  const setIndex = vi.fn();
  const { rerender } = renderHook(({ candidates }) => useSlashSelection(candidates, 0, setIndex, "agent:"), {
    initialProps: { candidates: [command, room] },
  });
  rerender({ candidates: [skill, command, room] });
  expect(setIndex).not.toHaveBeenCalled();
});

it("retains a selected suggested command when a skill batch is inserted before it", () => {
  const command: SlashPickerCandidate = { name: "创建房间", type: "command" };
  const first: SlashPickerCandidate = { name: "alpha", type: "skill" };
  const next: SlashPickerCandidate = { name: "beta", type: "skill" };
  const setIndex = vi.fn();
  const { rerender } = renderHook(({ candidates, scope }) => useSlashSelection(candidates, 1, setIndex, scope), {
    initialProps: { candidates: [first, command], scope: "agent:" },
  });
  rerender({ candidates: [first, next, command], scope: "agent:" });
  expect(setIndex).toHaveBeenCalledWith(2);
  setIndex.mockClear();
  rerender({ candidates: [first], scope: "agent:alpha" });
  expect(setIndex).not.toHaveBeenCalled();
});
