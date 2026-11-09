import { serializeTaskPromptMentions, updateTaskPromptMentions } from "@/pages/StartTaskPage/taskPromptMentions";

const mentions = [{ start: 2, end: 14, userId: "worker", userName: "Linda Smith" }];
const value = "请 @Linda Smith 检查";
const members = new Set(["worker"]);

it("keeps member identity through edits before and after a selected name", () => {
  const next = `现在${value}`;
  const moved = updateTaskPromptMentions(value, next, mentions);
  expect(serializeTaskPromptMentions(next, moved, members)).toBe('现在请 <at user_id="worker">Linda Smith</at> 检查');
  const appended = `${value}代码`;
  expect(serializeTaskPromptMentions(appended, updateTaskPromptMentions(value, appended, mentions), members)).toBe(
    '请 <at user_id="worker">Linda Smith</at> 检查代码',
  );
});

it("drops a reference when its visible name is edited or its member leaves the selected target", () => {
  const edited = value.replace("Linda", "Lina");
  expect(updateTaskPromptMentions(value, edited, mentions)).toEqual([]);
  expect(serializeTaskPromptMentions(value, mentions, new Set())).toBe(value);
  expect(serializeTaskPromptMentions(edited, mentions, members)).toBe(edited);
});

it("preserves the selected IDs for members with the same display name", () => {
  const mentions = [
    { start: 0, end: 4, userId: "one", userName: "Dev" },
    { start: 5, end: 9, userId: "two", userName: "Dev" },
  ];
  expect(serializeTaskPromptMentions("@Dev @Dev", mentions, new Set(["one", "two"]))).toBe(
    '<at user_id="one">Dev</at> <at user_id="two">Dev</at>',
  );
});
