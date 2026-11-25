import { feedbackAttachmentKind } from "@/models/feedback";

it.each([
  ["clipboard", "image/png", "image"],
  ["SCREENSHOT.JPG", "", "image"],
  ["diagnostics.7z", "", "archive"],
  ["diagnostics", "application/vnd.rar", "archive"],
  ["diagnostics.tar.gz", "application/octet-stream", "archive"],
  ["renamed.png", "image/svg+xml", undefined],
  ["zip", "", undefined],
] as const)("classifies %s with MIME type %s as %s", (name, type, expected) => {
  expect(feedbackAttachmentKind(new File(["a"], name, { type }))).toBe(expected);
});
