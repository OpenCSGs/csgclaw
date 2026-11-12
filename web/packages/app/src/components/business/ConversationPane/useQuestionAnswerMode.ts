import { useCallback, useEffect, useMemo, useState } from "react";
import { respondToUserInput, type UserInputAnswer } from "@/api/agentActivities";
import { errorMessage } from "@/api/client";
import { pendingQuestionActivities, questionOptions, type AgentActivityPayload } from "@/models/agentActivity";
import type { IMMessage, TranslateFn } from "@/models/conversations";

type DraftAnswer = {
  optionIndex?: number;
  skipped?: boolean;
};

type DraftAnswers = Record<string, DraftAnswer>;
type DraftTexts = Record<string, string>;

export type QuestionAnswerMode = {
  answers: DraftAnswers;
  busy: boolean;
  error: string;
  pending: AgentActivityPayload[];
  questionIndex: number;
  selected: AgentActivityPayload | null;
  text: string;
  chooseOption: (optionIndex: number) => void;
  closeRequest: () => void;
  nextQuestion: () => void;
  previousQuestion: () => void;
  select: (activityID: string, questionID?: string, optionIndex?: number) => void;
  setText: (value: string) => void;
  skipQuestion: () => void;
  submitAnswers: () => void;
};

export function useQuestionAnswerMode({
  messages,
  t,
}: {
  messages: readonly IMMessage[];
  responderID: string;
  roomID: string;
  t: TranslateFn;
}): QuestionAnswerMode {
  const allPending = useMemo(() => pendingQuestionActivities(messages), [messages]);
  const [settledIDs, setSettledIDs] = useState<Set<string>>(() => new Set());
  const pending = useMemo(
    () => allPending.filter((activity) => !settledIDs.has(activity.content.question?.id || "")),
    [allPending, settledIDs],
  );
  const pendingKey = pending.map((activity) => activity.content.question?.id || "").join("\n");
  const [selectedID, setSelectedID] = useState("");
  const [questionIndex, setQuestionIndex] = useState(0);
  const [answers, setAnswers] = useState<DraftAnswers>({});
  const [texts, setTexts] = useState<DraftTexts>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const selected = pending.find((activity) => activity.content.question?.id === selectedID) ?? null;
  const currentQuestion = selected?.content.question?.questions[questionIndex];
  const text = currentQuestion ? (texts[currentQuestion.id] ?? "") : "";

  useEffect(() => {
    const pendingIDs = new Set(allPending.map((activity) => activity.content.question?.id || ""));
    setSettledIDs((current) => {
      const next = new Set(Array.from(current).filter((id) => pendingIDs.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [allPending]);

  useEffect(() => {
    if (selectedID && pending.some((activity) => activity.content.question?.id === selectedID)) {
      return;
    }
    const nextSelectedID = pending.length === 1 ? pending[0]?.content.question?.id || "" : "";
    if (nextSelectedID === selectedID) {
      return;
    }
    setSelectedID(nextSelectedID);
    setQuestionIndex(0);
    setAnswers({});
    setTexts({});
    setError("");
  }, [pending, pendingKey, selectedID]);

  const resolve = useCallback(
    async (activity: AgentActivityPayload, responseAnswers: Record<string, UserInputAnswer>) => {
      const snapshot = activity.content.question;
      if (!snapshot?.id || busy) {
        return;
      }
      setBusy(true);
      setError("");
      try {
        const result = await respondToUserInput(activity.channel || snapshot.channel || "csgclaw", snapshot.id, {
          answers: responseAnswers,
        });
        if (result.status !== "pending") {
          setSettledIDs((current) => new Set(current).add(snapshot.id));
          setSelectedID("");
          setQuestionIndex(0);
          setAnswers({});
          setTexts({});
        }
      } catch (err) {
        setError(errorMessage(err, t("questionResponseFailed")));
      } finally {
        setBusy(false);
      }
    },
    [busy, t],
  );

  const previousQuestion = useCallback(() => {
    if (!busy) {
      setQuestionIndex((value) => Math.max(0, value - 1));
      setError("");
    }
  }, [busy]);

  const nextQuestion = useCallback(() => {
    const questionCount = selected?.content.question?.questions.length ?? 0;
    if (!busy) {
      setQuestionIndex((value) => Math.min(Math.max(0, questionCount - 1), value + 1));
      setError("");
    }
  }, [busy, selected]);

  const setText = useCallback(
    (value: string) => {
      if (!currentQuestion || busy) {
        return;
      }
      setTexts((current) => ({ ...current, [currentQuestion.id]: value }));
      setAnswers((current) => ({ ...current, [currentQuestion.id]: {} }));
      setError("");
    },
    [busy, currentQuestion],
  );

  const serializedAnswers = useCallback((): Record<string, UserInputAnswer> | null => {
    const snapshot = selected?.content.question;
    if (!snapshot) {
      return null;
    }
    const result: Record<string, UserInputAnswer> = {};
    for (let index = 0; index < snapshot.questions.length; index += 1) {
      const question = snapshot.questions[index];
      const draft = answers[question.id] ?? {};
      const note = (texts[question.id] ?? "").trim();
      if (draft.skipped) {
        result[question.id] = { answers: [] };
        continue;
      }
      if (draft.optionIndex) {
        const option = questionOptions(question)[draft.optionIndex - 1];
        if (!option) {
          setQuestionIndex(index);
          setError(t("questionAnswerRequired"));
          return null;
        }
        result[question.id] = { answers: [option.label] };
        continue;
      }
      if (note) {
        result[question.id] = { answers: [`user_note: ${note}`] };
        continue;
      }
      setQuestionIndex(index);
      setError(t("questionAnswerRequired"));
      return null;
    }
    return result;
  }, [answers, selected, t, texts]);

  const submitAnswers = useCallback(() => {
    if (!selected || busy) {
      return;
    }
    const response = serializedAnswers();
    if (response) {
      void resolve(selected, response);
    }
  }, [busy, resolve, selected, serializedAnswers]);

  const skipQuestion = useCallback(() => {
    const snapshot = selected?.content.question;
    const question = snapshot?.questions[questionIndex];
    if (!selected || !snapshot || !question || busy) {
      return;
    }
    const nextAnswers: DraftAnswers = { ...answers, [question.id]: { skipped: true } };
    const nextTexts = { ...texts, [question.id]: "" };
    setAnswers(nextAnswers);
    setTexts(nextTexts);
    setError("");
    if (questionIndex < snapshot.questions.length - 1) {
      setQuestionIndex((value) => value + 1);
      return;
    }
    const response: Record<string, UserInputAnswer> = {};
    for (const item of snapshot.questions) {
      const draft = nextAnswers[item.id] ?? {};
      const note = (nextTexts[item.id] ?? "").trim();
      if (draft.skipped) {
        response[item.id] = { answers: [] };
      } else if (draft.optionIndex) {
        const option = questionOptions(item)[draft.optionIndex - 1];
        if (!option) {
          return;
        }
        response[item.id] = { answers: [option.label] };
      } else if (note) {
        response[item.id] = { answers: [`user_note: ${note}`] };
      } else {
        response[item.id] = { answers: [] };
      }
    }
    void resolve(selected, response);
  }, [answers, busy, questionIndex, resolve, selected, texts]);

  const chooseOption = useCallback(
    (optionIndex: number) => {
      const snapshot = selected?.content.question;
      const question = snapshot?.questions[questionIndex];
      if (!selected || !snapshot || !question || busy) {
        return;
      }
      const option = questionOptions(question)[optionIndex - 1];
      if (!option) {
        return;
      }
      const nextAnswers: DraftAnswers = { ...answers, [question.id]: { optionIndex } };
      const nextTexts = { ...texts, [question.id]: "" };
      setAnswers(nextAnswers);
      setTexts(nextTexts);
      setError("");
      if (questionIndex < snapshot.questions.length - 1) {
        setQuestionIndex((value) => value + 1);
        return;
      }
      const response: Record<string, UserInputAnswer> = {};
      for (const item of snapshot.questions) {
        const draft = nextAnswers[item.id] ?? {};
        const note = (nextTexts[item.id] ?? "").trim();
        if (draft.skipped) {
          response[item.id] = { answers: [] };
        } else if (draft.optionIndex) {
          const selectedOption = questionOptions(item)[draft.optionIndex - 1];
          if (!selectedOption) {
            return;
          }
          response[item.id] = { answers: [selectedOption.label] };
        } else if (note) {
          response[item.id] = { answers: [`user_note: ${note}`] };
        } else {
          setQuestionIndex(snapshot.questions.indexOf(item));
          setError(t("questionAnswerRequired"));
          return;
        }
      }
      void resolve(selected, response);
    },
    [answers, busy, questionIndex, resolve, selected, t, texts],
  );

  const select = useCallback(
    (activityID: string, questionID?: string, optionIndex?: number) => {
      const activity = pending.find((item) => item.content.question?.id === activityID);
      if (!activity || busy) {
        return;
      }
      const questions = activity.content.question?.questions ?? [];
      const nextIndex = questionID
        ? Math.max(
            0,
            questions.findIndex((question) => question.id === questionID),
          )
        : 0;
      setSelectedID(activityID);
      setQuestionIndex(nextIndex);
      setAnswers(questionID && optionIndex ? { [questionID]: { optionIndex } } : {});
      setTexts({});
      setError("");
    },
    [busy, pending],
  );

  const closeRequest = useCallback(() => {
    if (selected && !busy) {
      void resolve(selected, {});
    }
  }, [busy, resolve, selected]);

  return {
    answers,
    busy,
    chooseOption,
    closeRequest,
    error,
    nextQuestion,
    pending,
    previousQuestion,
    questionIndex,
    select,
    selected,
    setText,
    skipQuestion,
    submitAnswers,
    text,
  };
}
