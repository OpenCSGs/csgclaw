package activity

import (
	"fmt"
	"strings"
)

const userInputMarkdownSeparator = "："

// RedactSecretUserInputResponse preserves the Codex response shape while
// replacing submitted secret values before they enter a persisted model
// session. Non-secret values and skipped secret arrays remain unchanged.
func RedactSecretUserInputResponse(snapshot UserInputSnapshot, response RequestUserInputResponse) RequestUserInputResponse {
	secretQuestions := make(map[string]bool, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		secretQuestions[question.ID] = question.IsSecret
	}
	redacted := RequestUserInputResponse{Answers: make(map[string]RequestUserInputAnswer, len(response.Answers))}
	for questionID, answer := range response.Answers {
		values := append([]string(nil), answer.Answers...)
		if secretQuestions[questionID] && len(values) > 0 {
			values = []string{"<redacted>"}
		}
		redacted.Answers[questionID] = RequestUserInputAnswer{Answers: values}
	}
	return redacted
}

// UserInputQuestionMarkdown renders a stable, readable transcript of an
// interactive request while structured state remains available in metadata.
func UserInputQuestionMarkdown(snapshot UserInputSnapshot) string {
	if len(snapshot.Questions) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("## Questions\n\n")
	for _, question := range snapshot.Questions {
		fmt.Fprintf(&out, "- %s%s%s\n", markdownLine(question.ID), userInputMarkdownSeparator, markdownLine(question.Question))
		for _, option := range question.Options {
			label := markdownLine(option.Label)
			description := markdownLine(option.Description)
			if description == "" {
				fmt.Fprintf(&out, "  - %s\n", label)
				continue
			}
			fmt.Fprintf(&out, "  - %s (%s)\n", label, description)
		}
	}
	status := userInputRequestStatusMarkdown(snapshot.Status)
	if status != "" {
		out.WriteByte('\n')
		out.WriteString(status)
		out.WriteByte('\n')
	}
	return strings.TrimSpace(out.String())
}

// UserInputAnswerMarkdown renders redacted answers in original question order.
func UserInputAnswerMarkdown(snapshot UserInputSnapshot) string {
	if len(snapshot.Answers) == 0 || len(snapshot.Questions) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("## Answers\n\n")
	for _, question := range snapshot.Questions {
		answer, ok := snapshot.Answers[question.ID]
		if !ok {
			continue
		}
		label, description := userInputAnswerLabelAndDescription(question, answer)
		fmt.Fprintf(&out, "- %s%s%s (%s)\n",
			markdownLine(question.ID),
			userInputMarkdownSeparator,
			markdownLine(label),
			markdownLine(description),
		)
	}
	return strings.TrimSpace(out.String())
}

func userInputAnswerLabelAndDescription(question UserInputQuestionSnapshot, answer UserInputAnswerSnapshot) (string, string) {
	switch {
	case answer.Skipped || !answer.Answered:
		return "Skipped", "No answer provided"
	case answer.Secret:
		return "Secret recorded", "Secret value redacted"
	}

	optionIndex := answer.OptionIndex - 1
	hasSourceOption := optionIndex >= 0 && optionIndex < len(question.Options)
	optionLabel := strings.TrimSpace(answer.OptionLabel)
	text := strings.TrimSpace(answer.Text)
	if !hasSourceOption && text != "" {
		return text, "Custom answer"
	}
	if optionLabel == "" && hasSourceOption {
		optionLabel = strings.TrimSpace(question.Options[optionIndex].Label)
	}
	if optionLabel == "" {
		optionLabel = text
		text = ""
	}
	if text != "" {
		optionLabel += "; " + text
	}
	description := "No description provided"
	if hasSourceOption {
		if source := strings.TrimSpace(question.Options[optionIndex].Description); source != "" {
			description = source
		}
	}
	return optionLabel, description
}

func userInputRequestStatusMarkdown(status UserInputStatus) string {
	switch status {
	case UserInputStatusSkipped:
		return "Status: Request skipped."
	case UserInputStatusExpired:
		return "Status: Request expired."
	case UserInputStatusCanceled:
		return "Status: Request canceled."
	case UserInputStatusInterrupted:
		return "Status: Request interrupted."
	default:
		return ""
	}
}

func markdownLine(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
