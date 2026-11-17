import { useState } from "react";
import { QuestionCard } from "@/components/ui";
import { createTranslator } from "@/shared/i18n";
import { InteractiveExample } from "./ApplicationFoundations/InteractiveExample";
import { useLabTranslation } from "./LabLocale";

export function QuestionCardPreview() {
  const { locale } = useLabTranslation();
  const zh = locale === "zh";

  return (
    <InteractiveExample
      id="question-card"
      title={zh ? "提问卡片" : "Question card"}
      description={
        zh
          ? "切换题目、选择答案，或点击「回答」查看自由回答的反馈。"
          : "Move between questions, pick an option, or use Answer to see the free-form answer feedback."
      }
    >
      <QuestionCardDemo />
    </InteractiveExample>
  );
}

function QuestionCardDemo() {
  const { locale } = useLabTranslation();
  const [selection, setSelection] = useState<{ id: string; index?: number }>();
  const t = createTranslator(locale);
  const zh = locale === "zh";
  const questions = [
    {
      id: "format",
      heading: zh ? "输出格式" : "Output format",
      prompt: zh ? "你希望如何查看这份报告？" : "How would you like to review the report?",
      options: [
        { label: zh ? "简短摘要" : "Short summary", description: zh ? "只列出要点" : "Include the key points" },
        {
          label: zh ? "完整报告" : "Full report",
          description: zh ? "包含细节和引用" : "Include details and references",
        },
      ],
    },
    { id: "notes", prompt: zh ? "还有什么需要补充的吗？" : "Is there anything else to include?", options: [] },
  ];
  const selected = questions.find(({ id }) => id === selection?.id);
  const answer = selection?.index === undefined ? null : selected?.options[selection.index];

  return (
    <>
      <QuestionCard
        title={t("questionRequest")}
        status={t("questionStatusPending")}
        questions={questions}
        labels={{
          answer: t("questionAnswer"),
          navigation: t("questionNavigation"),
          previous: t("questionPrevious"),
          next: t("questionNext"),
          progress: (current, total) => t("questionProgressCompact", { current, total }),
        }}
        selection={selection?.index === undefined ? null : { questionId: selection.id, optionIndex: selection.index }}
        onAnswer={(id, index) => setSelection({ id, index })}
      />
      {selected ? <p role="status">{answer?.label ?? t("questionFreeformAnswer")}</p> : null}
    </>
  );
}
