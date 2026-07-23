package activity

import (
	"strings"
	"testing"
	"time"
)

func TestUserInputQuestionMarkdown(t *testing.T) {
	t.Parallel()

	snapshot := UserInputSnapshot{
		ID:     "request-1",
		Status: UserInputStatusPending,
		Questions: []UserInputQuestionSnapshot{
			{
				ID: "demo_kind", Question: "What kind of CSGClaw demo should this be?",
				Options: []UserInputOptionSnapshot{
					{Label: "Bug fix (Recommended)", Description: "Plans a focused repair workflow."},
					{Label: "New feature", Description: "Plans a user-facing feature."},
				},
			},
			{ID: "freeform_note", Question: "Add a freeform note.", IsOther: true},
			{ID: "test_secret", Question: "Enter a disposable test value only.", IsOther: true, IsSecret: true},
		},
	}

	want := strings.Join([]string{
		"## Questions",
		"",
		"- demo_kind：What kind of CSGClaw demo should this be?",
		"  - Bug fix (Recommended) (Plans a focused repair workflow.)",
		"  - New feature (Plans a user-facing feature.)",
		"- freeform_note：Add a freeform note.",
		"- test_secret：Enter a disposable test value only.",
	}, "\n")
	if got := UserInputQuestionMarkdown(snapshot); got != want {
		t.Fatalf("UserInputQuestionMarkdown() = %q, want %q", got, want)
	}

	snapshot.Status = UserInputStatusExpired
	if got := UserInputQuestionMarkdown(snapshot); !strings.HasSuffix(got, "\n\nStatus: Request expired.") {
		t.Fatalf("expired markdown = %q, want status", got)
	}
}

func TestUserInputAnswerMarkdownUsesStrictReadableShape(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	snapshot := UserInputSnapshot{
		ID: "request-1", Status: UserInputStatusAnswered, ResolvedAt: &now,
		Questions: []UserInputQuestionSnapshot{
			{
				ID: "demo_kind", Question: "Choose",
				Options: []UserInputOptionSnapshot{{Label: "Bug fix (Recommended)", Description: "Plans a focused repair workflow."}},
			},
			{ID: "destination", Question: "Where?", IsOther: true},
			{ID: "combined", Question: "Choose and explain", Options: []UserInputOptionSnapshot{{Label: "Strict"}}},
			{ID: "freeform_note", Question: "Note"},
			{ID: "skipped_secret", Question: "Optional secret", IsSecret: true},
			{ID: "test_secret", Question: "Secret", IsSecret: true},
		},
		Answers: map[string]UserInputAnswerSnapshot{
			"demo_kind":      {Answered: true, OptionIndex: 1, OptionLabel: "Bug fix (Recommended)"},
			"destination":    {Answered: true, Text: "QA / 验收"},
			"combined":       {Answered: true, OptionIndex: 1, OptionLabel: "Strict", Text: "include edge cases"},
			"freeform_note":  {Skipped: true},
			"skipped_secret": {Skipped: true, Secret: true},
			"test_secret":    {Answered: true, Text: "******", Secret: true},
		},
	}

	want := strings.Join([]string{
		"## Answers",
		"",
		"- demo_kind：Bug fix (Recommended) (Plans a focused repair workflow.)",
		"- destination：QA / 验收 (Custom answer)",
		"- combined：Strict; include edge cases (No description provided)",
		"- freeform_note：Skipped (No answer provided)",
		"- skipped_secret：Skipped (No answer provided)",
		"- test_secret：Secret recorded (Secret value redacted)",
	}, "\n")
	if got := UserInputAnswerMarkdown(snapshot); got != want {
		t.Fatalf("UserInputAnswerMarkdown() = %q, want %q", got, want)
	}
}

func TestUserInputAnswerMarkdownEmptyWithoutSubmittedQuestionAnswers(t *testing.T) {
	t.Parallel()

	if got := UserInputAnswerMarkdown(UserInputSnapshot{Status: UserInputStatusSkipped}); got != "" {
		t.Fatalf("UserInputAnswerMarkdown() = %q, want empty", got)
	}
}

func TestRedactSecretUserInputResponsePreservesWireShape(t *testing.T) {
	t.Parallel()

	snapshot := UserInputSnapshot{Questions: []UserInputQuestionSnapshot{
		{ID: "plain"},
		{ID: "secret", IsSecret: true},
		{ID: "skipped_secret", IsSecret: true},
	}}
	response := RequestUserInputResponse{Answers: map[string]RequestUserInputAnswer{
		"plain":          {Answers: []string{"Alpha", "user_note: beta"}},
		"secret":         {Answers: []string{"user_note: disposable-secret"}},
		"skipped_secret": {Answers: []string{}},
	}}

	redacted := RedactSecretUserInputResponse(snapshot, response)
	if got := redacted.Answers["plain"].Answers; len(got) != 2 || got[0] != "Alpha" || got[1] != "user_note: beta" {
		t.Fatalf("plain answer = %#v", got)
	}
	if got := redacted.Answers["secret"].Answers; len(got) != 1 || got[0] != "<redacted>" {
		t.Fatalf("secret answer = %#v", got)
	}
	if got := redacted.Answers["skipped_secret"].Answers; len(got) != 0 {
		t.Fatalf("skipped secret answer = %#v", got)
	}
	if got := response.Answers["secret"].Answers[0]; got != "user_note: disposable-secret" {
		t.Fatalf("source response mutated to %q", got)
	}
}
