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
    content: JSON.stringify({
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
    }),
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

    act(() => {
      result.current.setText(" 2 ");
    });
    act(() => {
      result.current.nextQuestion();
    });
    expect(result.current.questionIndex).toBe(1);
    expect(mockedRespond).not.toHaveBeenCalled();

    act(() => {
      result.current.setText("matte finish");
    });
    act(() => {
      result.current.nextQuestion();
    });
    act(() => result.current.chooseOption(1));
    expect(result.current.questionIndex).toBe(2);
    expect(result.current.answers.finish).toEqual({ option_index: 1, skip: undefined });

    act(() => result.current.previousQuestion());
    expect(result.current.questionIndex).toBe(1);
    expect(result.current.text).toBe("matte finish");
    act(() => result.current.previousQuestion());
    expect(result.current.questionIndex).toBe(0);
    expect(result.current.text).toBe(" 2 ");
    act(() => result.current.setText("1"));
    act(() => result.current.nextQuestion());
    expect(result.current.text).toBe("matte finish");
    act(() => result.current.nextQuestion());
    expect(result.current.answers.finish).toEqual({ option_index: 1, skip: undefined });
    expect(mockedRespond).not.toHaveBeenCalled();

    act(() => result.current.submitAnswers());
    await waitFor(() => expect(mockedRespond).toHaveBeenCalledTimes(1));
    expect(mockedRespond).toHaveBeenCalledWith("csgclaw", "request-1", {
      answers: {
        color: { option_index: 1, text: undefined },
        detail: { option_index: undefined, text: "matte finish" },
        finish: { option_index: 1, text: undefined },
      },
      responder_id: "user-admin",
      room_id: "room-1",
      skip_all: undefined,
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

  it("advances after an option answer but stays for Other detail", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());

    act(() => result.current.chooseOption(2));
    expect(result.current.answers.color).toEqual({ option_index: 2, skip: undefined });
    expect(result.current.questionIndex).toBe(1);

    act(() => result.current.previousQuestion());
    act(() => result.current.chooseOption(3));
    expect(result.current.answers.color).toEqual({ option_index: 3, skip: undefined });
    expect(result.current.questionIndex).toBe(0);
  });

  it("clears an option when the selected choice is clicked again", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());

    act(() => result.current.nextQuestion());
    act(() => result.current.nextQuestion());
    act(() => result.current.chooseOption(2));
    expect(result.current.answers.finish).toEqual({ option_index: 2, skip: undefined });

    act(() => result.current.chooseOption(2));
    expect(result.current.answers.finish).toBeUndefined();
    expect(result.current.questionIndex).toBe(2);
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
    expect(result.current.questionIndex).toBe(1);
    expect(result.current.answers.color).toEqual({ option_index: 2 });
  });

  it("submits an empty map only after confirmed skip-all", async () => {
    const { result } = renderHook(() =>
      useQuestionAnswerMode({ messages: [questionMessage()], responderID: "user-admin", roomID: "room-1", t }),
    );
    await waitFor(() => expect(result.current.selected).not.toBeNull());
    act(() => result.current.continueWithoutAnswering());
    await waitFor(() => expect(mockedRespond).toHaveBeenCalledTimes(1));
    expect(mockedRespond.mock.calls[0]?.[2]).toEqual({
      answers: undefined,
      responder_id: "user-admin",
      room_id: "room-1",
      skip_all: true,
    });
  });
});
