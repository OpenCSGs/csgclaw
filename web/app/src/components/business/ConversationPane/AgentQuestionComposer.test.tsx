import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AgentActivityPayload } from "@/models/agentActivity";
import type { TranslateFn } from "@/models/conversations";
import { AgentQuestionComposer } from "./AgentQuestionComposer";
import type { QuestionAnswerMode } from "./useQuestionAnswerMode";

const t: TranslateFn = (key, params) => {
  if (key === "questionProgress") {
    return `Question ${params?.current} of ${params?.total}`;
  }
  return key;
};

function activity(secret = false): AgentActivityPayload {
  return {
    channel: "csgclaw",
    event_id: "event-1",
    origin_server_ts: 1,
    room_id: "room-1",
    sender: "Agent One",
    type: "com.opencsg.csgclaw.agent.activity",
    version: 1,
    content: {
      body: "Question pending",
      msgtype: "com.opencsg.csgclaw.agent.question",
      question: {
        id: "request-1",
        questions: [
          {
            header: "Choice",
            id: "choice",
            is_other: true,
            is_secret: secret,
            options: [{ label: "First", description: "The first option" }],
            question: "Pick one",
          },
        ],
        status: "pending",
      },
    },
  };
}

function multiQuestionActivity(): AgentActivityPayload {
  const first = activity();
  return {
    ...first,
    content: {
      ...first.content,
      question: {
        ...first.content.question!,
        questions: [
          ...first.content.question!.questions,
          {
            header: "Detail",
            id: "detail",
            options: [],
            question: "Add detail",
          },
        ],
      },
    },
  };
}

function mode(overrides: Partial<QuestionAnswerMode> = {}): QuestionAnswerMode {
  const selected = activity();
  return {
    answers: {},
    busy: false,
    chooseOption: vi.fn(),
    continueWithoutAnswering: vi.fn(),
    error: "",
    nextQuestion: vi.fn(),
    pending: [selected],
    previousQuestion: vi.fn(),
    questionIndex: 0,
    select: vi.fn(),
    selected,
    setSkipConfirmation: vi.fn(),
    setText: vi.fn(),
    skipConfirmation: false,
    skipQuestion: vi.fn(),
    submitAnswers: vi.fn(),
    text: "",
    ...overrides,
  };
}

describe("AgentQuestionComposer", () => {
  it("supports click selection and Enter submission", async () => {
    const user = userEvent.setup();
    const answerMode = mode();
    render(<AgentQuestionComposer mode={answerMode} t={t} />);

    await user.click(screen.getByRole("radio", { name: /First/ }));
    expect(answerMode.chooseOption).toHaveBeenCalledWith(1);
    await user.type(screen.getByRole("textbox"), "2{Enter}");
    expect(answerMode.setText).toHaveBeenLastCalledWith("2");
    expect(answerMode.submitAnswers).toHaveBeenCalled();
  });

  it("provides previous and next arrow navigation for one multi-question request", async () => {
    const user = userEvent.setup();
    const selected = multiQuestionActivity();
    const answerMode = mode({ pending: [selected], selected });
    render(<AgentQuestionComposer mode={answerMode} t={t} />);

    expect(screen.getByRole("button", { name: "questionPrevious" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "questionNext" }));
    expect(answerMode.nextQuestion).toHaveBeenCalledOnce();
    expect(answerMode.submitAnswers).not.toHaveBeenCalled();
  });

  it("uses a password field for secret questions", () => {
    const secret = activity(true);
    render(<AgentQuestionComposer mode={mode({ pending: [secret], selected: secret })} t={t} />);
    expect(screen.getByLabelText("questionSecretAnswer")).toHaveAttribute("type", "password");
  });

  it("requires explicit selection when several agents are waiting", async () => {
    const user = userEvent.setup();
    const first = activity();
    const second = {
      ...activity(),
      sender: "Agent Two",
      content: { ...activity().content, question: { ...activity().content.question!, id: "request-2" } },
    };
    const answerMode = mode({ pending: [first, second], selected: null });
    render(<AgentQuestionComposer mode={answerMode} t={t} />);

    await user.click(screen.getByRole("button", { name: /Agent Two/ }));
    expect(answerMode.select).toHaveBeenCalledWith("request-2");
  });

  it("confirms before continuing without answers", async () => {
    const user = userEvent.setup();
    const answerMode = mode({ skipConfirmation: true });
    render(<AgentQuestionComposer mode={answerMode} t={t} />);
    await user.click(screen.getByRole("button", { name: "questionConfirmContinue" }));
    expect(answerMode.continueWithoutAnswering).toHaveBeenCalled();
  });
});
