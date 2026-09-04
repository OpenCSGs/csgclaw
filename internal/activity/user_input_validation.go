package activity

import (
	"fmt"
	"strings"
)

func NormalizeUserInputQuestions(questions []UserInputQuestionSnapshot) ([]UserInputQuestionSnapshot, error) {
	if len(questions) < 1 || len(questions) > 32 {
		return nil, fmt.Errorf("%w: expected 1 to %d questions", ErrUserInputInvalidResponse, 32)
	}
	seen := make(map[string]struct{}, len(questions))
	out := make([]UserInputQuestionSnapshot, 0, len(questions))
	for _, question := range questions {
		question.ID = strings.TrimSpace(question.ID)
		question.Header = strings.TrimSpace(question.Header)
		question.Question = strings.TrimSpace(question.Question)
		if question.ID == "" || question.Header == "" || question.Question == "" {
			return nil, fmt.Errorf("%w: question id, header, and text are required", ErrUserInputInvalidResponse)
		}
		if _, ok := seen[question.ID]; ok {
			return nil, fmt.Errorf("%w: duplicate question id %q", ErrUserInputInvalidResponse, question.ID)
		}
		seen[question.ID] = struct{}{}
		if len(question.Options) > 12 {
			return nil, fmt.Errorf("%w: question %q has more than %d options", ErrUserInputInvalidResponse, question.ID, 12)
		}
		options := make([]UserInputOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				return nil, fmt.Errorf("%w: option labels are required", ErrUserInputInvalidResponse)
			}
			options = append(options, option)
		}
		question.Options = options
		out = append(out, question)
	}
	return out, nil
}

func BuildUserInputResponse(questions []UserInputQuestionSnapshot, response RequestUserInputResponse) (UserInputStatus, RequestUserInputResponse, map[string]UserInputAnswerSnapshot, error) {
	if len(response.Answers) == 0 {
		return UserInputStatusSkipped, RequestUserInputResponse{Answers: map[string]RequestUserInputAnswer{}}, nil, nil
	}
	known := make(map[string]UserInputQuestionSnapshot, len(questions))
	for _, question := range questions {
		known[question.ID] = question
	}
	for id := range response.Answers {
		if _, ok := known[id]; !ok {
			return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: unknown question id %q", ErrUserInputInvalidResponse, id)
		}
	}
	normalized := RequestUserInputResponse{Answers: make(map[string]RequestUserInputAnswer, len(questions))}
	snapshots := make(map[string]UserInputAnswerSnapshot, len(questions))
	answered := false
	for _, question := range questions {
		input, ok := response.Answers[question.ID]
		if !ok {
			return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: missing answer for question %q", ErrUserInputInvalidResponse, question.ID)
		}
		if len(input.Answers) == 0 {
			normalized.Answers[question.ID] = RequestUserInputAnswer{Answers: []string{}}
			snapshots[question.ID] = UserInputAnswerSnapshot{Skipped: true, Secret: question.IsSecret}
			continue
		}
		if len(input.Answers) > 2 {
			return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: question %q accepts at most one option and one user note", ErrUserInputInvalidResponse, question.ID)
		}
		values := make([]string, 0, len(input.Answers))
		snapshot := UserInputAnswerSnapshot{Answered: true, Secret: question.IsSecret}
		for _, rawValue := range input.Answers {
			value := strings.TrimSpace(rawValue)
			if value == "" {
				return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains an empty answer", ErrUserInputInvalidResponse, question.ID)
			}
			if strings.HasPrefix(value, "user_note:") {
				if snapshot.Text != "" {
					return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains multiple user notes", ErrUserInputInvalidResponse, question.ID)
				}
				note := strings.TrimSpace(strings.TrimPrefix(value, "user_note:"))
				if note == "" {
					return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains an empty user note", ErrUserInputInvalidResponse, question.ID)
				}
				values = append(values, "user_note: "+note)
				if question.IsSecret {
					snapshot.Text = "******"
				} else {
					snapshot.Text = note
				}
				continue
			}
			if snapshot.OptionIndex != 0 {
				return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: question %q contains multiple option labels", ErrUserInputInvalidResponse, question.ID)
			}
			optionIndex := 0
			for index, option := range question.Options {
				if value == option.Label {
					optionIndex = index + 1
					break
				}
			}
			if optionIndex == 0 && question.IsOther && value == "None of the above" {
				optionIndex = len(question.Options) + 1
			}
			if optionIndex == 0 {
				return "", RequestUserInputResponse{}, nil, fmt.Errorf("%w: unknown option label %q for question %q", ErrUserInputInvalidResponse, value, question.ID)
			}
			values = append(values, value)
			snapshot.OptionIndex = optionIndex
			snapshot.OptionLabel = value
		}
		normalized.Answers[question.ID] = RequestUserInputAnswer{Answers: values}
		snapshots[question.ID] = snapshot
		answered = true
	}
	status := UserInputStatusSkipped
	if answered {
		status = UserInputStatusAnswered
	}
	return status, normalized, snapshots, nil
}

func PublicUserInputSnapshot(snapshot UserInputSnapshot) UserInputSnapshot {
	out := snapshot
	out.Questions = append([]UserInputQuestionSnapshot(nil), snapshot.Questions...)
	for i := range out.Questions {
		out.Questions[i].Options = append([]UserInputOptionSnapshot(nil), snapshot.Questions[i].Options...)
	}
	if snapshot.Answers != nil {
		out.Answers = make(map[string]UserInputAnswerSnapshot, len(snapshot.Answers))
		for id, answer := range snapshot.Answers {
			if answer.Secret && answer.Answered && answer.Text != "" {
				answer.Text = "******"
			}
			out.Answers[id] = answer
		}
	}
	return out
}
