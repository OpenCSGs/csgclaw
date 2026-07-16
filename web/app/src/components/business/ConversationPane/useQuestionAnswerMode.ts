import { useCallback, useEffect, useMemo, useState } from "react";
import { respondToUserInput, type UserInputAnswer } from "@/api/agentActivities";
import { errorMessage } from "@/api/client";
import { pendingQuestionActivities, questionOptions, type AgentActivityPayload } from "@/models/agentActivity";
import type { IMMessage, TranslateFn } from "@/models/conversations";

type DraftAnswers = Record<string, UserInputAnswer>;
type DraftTexts = Record<string, string>;

export type QuestionAnswerMode = {
  answers: DraftAnswers;
  busy: boolean;
  error: string;
  pending: AgentActivityPayload[];
  questionIndex: number;
  selected: AgentActivityPayload | null;
  skipConfirmation: boolean;
  text: string;
  chooseOption: (optionIndex: number) => void;
  continueWithoutAnswering: () => void;
  nextQuestion: () => void;
  previousQuestion: () => void;
  select: (activityID: string, questionID?: string, optionIndex?: number) => void;
  setSkipConfirmation: (value: boolean) => void;
  setText: (value: string) => void;
  skipQuestion: () => void;
  submitAnswers: () => void;
};

export function useQuestionAnswerMode({
  messages,
  responderID,
  roomID,
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
  const [skipConfirmation, setSkipConfirmation] = useState(false);
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
    setSkipConfirmation(false);
  }, [pending, pendingKey, selectedID]);

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
      const selectedQuestion = questions[nextIndex];
      const selectedOther =
        Boolean(optionIndex) &&
        selectedQuestion?.is_other === true &&
        optionIndex === questionOptions(selectedQuestion).length;
      const shouldAdvance = Boolean(optionIndex) && !selectedOther && nextIndex < questions.length - 1;
      setSelectedID(activityID);
      setQuestionIndex(shouldAdvance ? nextIndex + 1 : nextIndex);
      setAnswers(
        questionID && optionIndex && questions[nextIndex]
          ? { [questions[nextIndex].id]: { option_index: optionIndex } }
          : {},
      );
      setTexts({});
      setError("");
      setSkipConfirmation(false);
    },
    [busy, pending],
  );

  const resolve = useCallback(
    async (activity: AgentActivityPayload, responseAnswers: DraftAnswers, skipAll = false) => {
      const snapshot = activity.content.question;
      if (!snapshot?.id || !roomID || !responderID || busy) {
        return;
      }
      setBusy(true);
      setError("");
      try {
        const result = await respondToUserInput(activity.channel || snapshot.channel || "csgclaw", snapshot.id, {
          answers: skipAll ? undefined : responseAnswers,
          responder_id: responderID,
          room_id: roomID,
          skip_all: skipAll || undefined,
        });
        if (result.status !== "pending") {
          setSettledIDs((current) => new Set(current).add(snapshot.id));
          setSelectedID("");
          setQuestionIndex(0);
          setAnswers({});
          setTexts({});
          setSkipConfirmation(false);
        }
      } catch (err) {
        setError(errorMessage(err, t("questionResponseFailed")));
      } finally {
        setBusy(false);
      }
    },
    [busy, responderID, roomID, t],
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
      setAnswers((current) => {
        if (!current[currentQuestion.id]?.skip) {
          return current;
        }
        return { ...current, [currentQuestion.id]: {} };
      });
      setError("");
    },
    [busy, currentQuestion],
  );

  const submitAnswers = useCallback(() => {
    const snapshot = selected?.content.question;
    if (!selected || !snapshot || busy) {
      return;
    }
    const nextAnswers: DraftAnswers = {};
    for (let index = 0; index < snapshot.questions.length; index += 1) {
      const question = snapshot.questions[index];
      const current = answers[question.id] ?? {};
      if (current.skip) {
        nextAnswers[question.id] = { skip: true };
        continue;
      }
      const trimmed = (texts[question.id] ?? "").trim();
      const availableOptions = questionOptions(question);
      let optionIndex = current.option_index;
      let note = trimmed;
      if (!optionIndex && /^\d+$/.test(trimmed)) {
        const numeric = Number(trimmed);
        if (numeric >= 1 && numeric <= availableOptions.length) {
          optionIndex = numeric;
          note = "";
        }
      }
      if (!optionIndex && !note) {
        setQuestionIndex(index);
        setError(t("questionAnswerRequired"));
        return;
      }
      if (optionIndex === availableOptions.length && question.is_other && !note) {
        setQuestionIndex(index);
        setError(t("questionOtherDetailRequired"));
        return;
      }
      nextAnswers[question.id] = {
        option_index: optionIndex,
        text: note || undefined,
      };
    }
    setAnswers(nextAnswers);
    setError("");
    void resolve(selected, nextAnswers);
  }, [answers, busy, resolve, selected, t, texts]);

  const skipQuestion = useCallback(() => {
    const snapshot = selected?.content.question;
    const question = snapshot?.questions[questionIndex];
    if (!selected || !snapshot || !question || busy) {
      return;
    }
    const nextAnswers = { ...answers, [question.id]: { skip: true } };
    setAnswers(nextAnswers);
    setTexts((current) => ({ ...current, [question.id]: "" }));
    setError("");
    if (questionIndex < snapshot.questions.length - 1) {
      setQuestionIndex((value) => value + 1);
    }
  }, [answers, busy, questionIndex, selected]);

  const chooseOption = useCallback(
    (optionIndex: number) => {
      const question = selected?.content.question?.questions[questionIndex];
      if (!question || busy) {
        return;
      }
      const deselect = answers[question.id]?.option_index === optionIndex;
      setAnswers((current) => {
        if (deselect) {
          const next = { ...current };
          delete next[question.id];
          return next;
        }
        return {
          ...current,
          [question.id]: { ...current[question.id], option_index: optionIndex, skip: undefined },
        };
      });
      setError("");
      if (deselect) {
        return;
      }
      const selectedOther = question.is_other && optionIndex === questionOptions(question).length;
      if (!selectedOther && questionIndex < (selected?.content.question?.questions.length ?? 0) - 1) {
        setQuestionIndex(questionIndex + 1);
      }
    },
    [answers, busy, questionIndex, selected],
  );

  const continueWithoutAnswering = useCallback(() => {
    if (selected) {
      void resolve(selected, {}, true);
    }
  }, [resolve, selected]);

  return {
    answers,
    busy,
    chooseOption,
    continueWithoutAnswering,
    error,
    nextQuestion,
    pending,
    previousQuestion,
    questionIndex,
    select,
    selected,
    setSkipConfirmation,
    setText,
    skipConfirmation,
    skipQuestion,
    submitAnswers,
    text,
  };
}
