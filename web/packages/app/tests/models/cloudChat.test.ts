import { expect, it } from "vitest";
import { emptyCloudChat, parseCloudChat, cloudResponseText } from "@/models/cloudChat";
it("recovers valid local history and rejects malformed storage", () => {
  expect(parseCloudChat("invalid")).toEqual(emptyCloudChat());
  expect(
    parseCloudChat(
      JSON.stringify({
        messages: [null, { id: "1", role: "user", text: "hello", token: "ignored" }],
        previousResponseID: "resp_done",
        pending: null,
      }),
    ),
  ).toEqual({ messages: [{ id: "1", role: "user", text: "hello" }], previousResponseID: "resp_done", pending: null });
});
it("reads only output text from a relay response", () => {
  expect(
    cloudResponseText({
      id: "r",
      model: "a",
      status: "completed",
      output: [
        {
          content: [
            { type: "output_text", text: "hello" },
            { type: "other", text: "hidden" },
          ],
        },
      ],
    }),
  ).toBe("hello");
});
