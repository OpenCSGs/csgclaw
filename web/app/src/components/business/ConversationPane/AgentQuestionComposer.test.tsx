import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentActivityPayload } from "@/models/agentActivity";
import type { TranslateFn } from "@/models/conversations";
import { AgentQuestionComposer } from "./AgentQuestionComposer";
import type { QuestionAnswerMode } from "./useQuestionAnswerMode";

const t: TranslateFn = (key, params) => {
  if (key === "questionProgress") {
    return `Question ${params?.current} of ${params?.total}`;
  }
  if (key === "questionExpiresIn") {
    return `Expires in ${params?.time}`;
  }
  return key;
};

afterEach(() => {
  vi.useRealTimers();
});

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
    closeRequest: vi.fn(),
    error: "",
    nextQuestion: vi.fn(),
    pending: [selected],
    previousQuestion: vi.fn(),
    questionIndex: 0,
    select: vi.fn(),
    selected,
    setText: vi.fn(),
    skipQuestion: vi.fn(),
    submitAnswers: vi.fn(),
    text: "",
    ...overrides,
  };
}

describe("AgentQuestionComposer", () => {
  it("selects a final option immediately and never renders None of the above", async () => {
    const user = userEvent.setup();
    const answerMode = mode();
    render(<AgentQuestionComposer mode={answerMode} t={t} />);

    await user.click(screen.getByRole("radio", { name: /First/ }));
    expect(answerMode.chooseOption).toHaveBeenCalledWith(1);
    expect(screen.queryByText("None of the above")).not.toBeInTheDocument();
  });

  it("renders a Recommended badge while keeping the full source label selectable", () => {
    const selected = activity();
    selected.content.question!.questions[0].options = [
      { label: "Best choice (Recommended)", description: "Use this first." },
    ];
    render(<AgentQuestionComposer mode={mode({ pending: [selected], selected })} t={t} />);

    expect(screen.getByText("Best choice")).toBeInTheDocument();
    expect(screen.getByText("questionRecommended")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Best choice.*Use this first/ })).toBeInTheDocument();
  });

  it("shows Skip for empty freeform text and Next after text is entered", async () => {
    const user = userEvent.setup();
    const answerMode = mode();
    const { rerender } = render(<AgentQuestionComposer mode={answerMode} t={t} />);

    expect(screen.getByRole("button", { name: "questionSkip" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox"), "custom");
    expect(answerMode.setText).toHaveBeenLastCalledWith("m");

    const textMode = mode({ text: "custom" });
    rerender(<AgentQuestionComposer mode={textMode} t={t} />);
    const nextButton = screen
      .getAllByRole("button", { name: "questionNext" })
      .find((button) => !button.hasAttribute("disabled"));
    expect(nextButton).toBeDefined();
    await user.click(nextButton!);
    expect(textMode.submitAnswers).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "questionSkip" })).not.toBeInTheDocument();
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

  it("preserves navigation controls for a five-question request", () => {
    const selected = activity();
    selected.content.question!.questions = Array.from({ length: 5 }, (_, index) => ({
      header: `Question ${index + 1}`,
      id: `q-${index + 1}`,
      options: index === 4 ? [] : [{ label: `Choice ${index + 1}` }],
      question: `Concrete question ${index + 1}?`,
    }));
    render(
      <AgentQuestionComposer
        mode={mode({ pending: [selected], questionIndex: 3, selected })}
        t={(key, params) => (key === "questionProgressCompact" ? `${params?.current} of ${params?.total}` : key)}
      />,
    );

    expect(screen.getByRole("heading", { name: "Concrete question 4?" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("4 of 5");
    expect(screen.getByRole("button", { name: "questionPrevious" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "questionNext" })).toBeEnabled();
  });

  it("counts down from the server-provided expiration deadline", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-21T08:00:00.000Z"));
    const selected = activity();
    selected.content.question!.auto_resolve_at = "2026-07-21T08:02:05.000Z";

    render(<AgentQuestionComposer mode={mode({ pending: [selected], selected })} t={t} />);

    expect(screen.getByRole("timer", { name: "Expires in 2:05" })).toHaveTextContent("2:05");
    act(() => vi.advanceTimersByTime(1_000));
    expect(screen.getByRole("timer", { name: "Expires in 2:04" })).toHaveTextContent("2:04");
    act(() => vi.advanceTimersByTime(124_000));
    expect(screen.getByRole("timer", { name: "Expires in 0:00" })).toHaveTextContent("0:00");
  });

  it("omits the countdown when the request has no valid deadline", () => {
    const selected = activity();
    const { rerender } = render(<AgentQuestionComposer mode={mode({ pending: [selected], selected })} t={t} />);

    expect(screen.queryByRole("timer")).not.toBeInTheDocument();
    selected.content.question!.auto_resolve_at = "not-a-date";
    rerender(<AgentQuestionComposer mode={mode({ pending: [selected], selected })} t={t} />);
    expect(screen.queryByRole("timer")).not.toBeInTheDocument();
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

  it("closes the entire request directly", async () => {
    const user = userEvent.setup();
    const answerMode = mode();
    render(<AgentQuestionComposer mode={answerMode} t={t} />);
    await user.click(screen.getByRole("button", { name: "questionClose" }));
    expect(answerMode.closeRequest).toHaveBeenCalledOnce();
  });
});
