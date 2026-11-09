import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { respondToUserInput } from "@/api/agentActivities";
import type { IMMessage, TranslateFn } from "@/models/conversations";
import { useQuestionAnswerMode } from "./useQuestionAnswerMode";

vi.mock("@/api/agentActivities", () => ({
  respondToUserInput: vi.fn(),
}));

const t: TranslateFn = (key) => key;
const mockedRespond = vi.mocked(respondToUserInput);

function questionMessage(id = "request-1"): IMMessage {
  return {
    id: `message-${id}`,
    content: "## Questions\n\n- color：Choose a color",
    metadata: {
      csgclaw: {
        agent_activity: {
          type: "com.opencsg.csgclaw.agent.activity",
          version: 1,
          event_id: `question-${id}`,
          sender: "Agent One",
          channel: "csgclaw",
          room_id: "room-1",
          origin_server_ts: 1,
          content: {
            msgtype: "com.opencsg.csgclaw.agent.question",
            body: "Question pending",
            question: {
              id,
              status: "pending",
              questions: [
                {
                  id: "color",
                  header: "Color",
                  is_other: true,
                  question: "Choose a color",
                  options: [{ label: "Blue" }, { label: "Green" }],
                },
                {
                  id: "detail",
                  header: "Detail",
                  question: "Add detail",
                  options: [],
                },
                {
                  id: "finish",
                  header: "Finish",
                  question: "Choose a finish",
                  options: [{ label: "Glossy" }, { label: "Matte" }],
                },
              ],
            },
          },
        },
      },
    },
  };
}

describe("useQuestionAnswerMode", () => {
  beforeEach(() => {
    mockedRespond.mockReset();
    mockedRespond.mockResolvedValue({ id: "request-1", status: "answered", questions: [] });
  });

  it("retains all drafts while navigating and submits every answer once", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected?.content.question?.id).toBe("request-1"));

    act(() => result.current.chooseOption(2));
    expect(result.current.questionIndex).toBe(1);
    expect(result.current.answers.color).toEqual({ optionIndex: 2 });

    act(() => {
      result.current.setText("matte finish");
    });
    act(() => {
      result.current.nextQuestion();
    });
    act(() => result.current.previousQuestion());
    expect(result.current.questionIndex).toBe(1);
    expect(result.current.text).toBe("matte finish");
    act(() => result.current.previousQuestion());
    expect(result.current.questionIndex).toBe(0);
    expect(result.current.answers.color).toEqual({ optionIndex: 2 });
    act(() => result.current.nextQuestion());
    expect(result.current.text).toBe("matte finish");
    act(() => result.current.nextQuestion());
    expect(mockedRespond).not.toHaveBeenCalled();

    act(() => result.current.chooseOption(1));
    await waitFor(() => expect(mockedRespond).toHaveBeenCalledTimes(1));
    expect(mockedRespond).toHaveBeenCalledWith("csgclaw", "request-1", {
      answers: {
        color: { answers: ["Green"] },
        detail: { answers: ["user_note: matte finish"] },
        finish: { answers: ["Glossy"] },
      },
    });
  });

  it("moves to the first unanswered question instead of partially submitting", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());

    act(() => result.current.nextQuestion());
    act(() => result.current.setText("detail"));
    act(() => result.current.nextQuestion());
    act(() => result.current.chooseOption(2));
    act(() => result.current.submitAnswers());

    expect(mockedRespond).not.toHaveBeenCalled();
    expect(result.current.questionIndex).toBe(0);
    expect(result.current.error).toBe("questionAnswerRequired");
  });

  it("keeps options and freeform text mutually exclusive", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());

    act(() => result.current.chooseOption(2));
    expect(result.current.answers.color).toEqual({ optionIndex: 2 });
    expect(result.current.questionIndex).toBe(1);

    act(() => result.current.previousQuestion());
    act(() => result.current.setText("custom blue"));
    expect(result.current.answers.color).toEqual({});
    expect(result.current.text).toBe("custom blue");
    expect(result.current.questionIndex).toBe(0);
  });

  it("skips individual questions with empty answer arrays", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());

    act(() => result.current.skipQuestion());
    expect(result.current.answers.color).toEqual({ skipped: true });
    expect(result.current.questionIndex).toBe(1);
  });

  it("does not guess when several agents are waiting", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({
        messages: [questionMessage("request-1"), questionMessage("request-2")],
        responderID: "user-admin",
        roomID: "room-1",
        t,
      }),
    );
    await waitFor(() => expect(result.current.pending).toHaveLength(2));
    expect(result.current.selected).toBeNull();
    act(() => result.current.select("request-2", "color", 2));
    expect(result.current.selected?.content.question?.id).toBe("request-2");
    expect(result.current.questionIndex).toBe(0);
    expect(result.current.answers.color).toEqual({ optionIndex: 2 });
  });

  it("submits an empty map when the request is closed", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());
    act(() => result.current.closeRequest());
    await waitFor(() => expect(mockedRespond).toHaveBeenCalledTimes(1));
    expect(mockedRespond.mock.calls[0]?.[2]).toEqual({ answers: {} });
  });
});
