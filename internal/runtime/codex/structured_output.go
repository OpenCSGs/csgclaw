package codex

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"csgclaw/internal/activity"
)

const (
	structuredOutputPrefix             = "::csgclaw-output::"
	structuredOutputRequestUserInput   = "request_user_input"
	structuredOutputResourceLink       = "resource_link"
	maxStructuredOutputRecordBytes     = 256 * 1024
	maxStructuredOutputQuestions       = 32
	maxStructuredOutputQuestionOptions = 12
	maxStructuredOutputResourceLinks   = 16
)

type structuredOutputDecoder func(json.RawMessage) (any, error)

var structuredOutputDecoders = map[string]structuredOutputDecoder{
	structuredOutputRequestUserInput: decodeRequestUserInputRecord,
	structuredOutputResourceLink:     decodeResourceLinkRecord,
}

func decodeStructuredCommandOutput(output string) (string, activity.StructuredOutputArtifact, []error) {
	var artifact activity.StructuredOutputArtifact
	var decodeErrors []error
	seenLinks := make(map[string]struct{})
	lines := strings.Split(output, "\n")
	kept := make([]string, 0, len(lines))

	for _, original := range lines {
		line := strings.TrimSuffix(original, "\r")
		if !strings.HasPrefix(line, structuredOutputPrefix) {
			kept = append(kept, original)
			continue
		}
		if len(line) > maxStructuredOutputRecordBytes {
			decodeErrors = append(decodeErrors, fmt.Errorf("structured output record exceeds %d bytes", maxStructuredOutputRecordBytes))
			kept = append(kept, original)
			continue
		}

		rest := strings.TrimPrefix(line, structuredOutputPrefix)
		kind, payload, ok := strings.Cut(rest, " ")
		kind = strings.TrimSpace(kind)
		payload = strings.TrimSpace(payload)
		decoder := structuredOutputDecoders[kind]
		if !ok || payload == "" || decoder == nil {
			kept = append(kept, original)
			continue
		}

		decoded, err := decoder(json.RawMessage(payload))
		if err != nil {
			decodeErrors = append(decodeErrors, fmt.Errorf("decode %s structured output: %w", kind, err))
			kept = append(kept, original)
			continue
		}

		switch value := decoded.(type) {
		case activity.RequestUserInputArgs:
			if artifact.RequestUserInput != nil {
				decodeErrors = append(decodeErrors, fmt.Errorf("only one request_user_input record is allowed per command output"))
				continue
			}
			artifact.RequestUserInput = &value
		case activity.ResourceLink:
			if _, exists := seenLinks[value.URI]; exists {
				continue
			}
			if len(artifact.ResourceLinks) >= maxStructuredOutputResourceLinks {
				decodeErrors = append(decodeErrors, fmt.Errorf("at most %d resource_link records are allowed", maxStructuredOutputResourceLinks))
				continue
			}
			seenLinks[value.URI] = struct{}{}
			artifact.ResourceLinks = append(artifact.ResourceLinks, value)
		}
	}

	return strings.Join(kept, "\n"), artifact, decodeErrors
}

func decodeRequestUserInputRecord(payload json.RawMessage) (any, error) {
	var args activity.RequestUserInputArgs
	if err := json.Unmarshal(payload, &args); err != nil {
		return nil, err
	}
	if len(args.Questions) < 1 || len(args.Questions) > maxStructuredOutputQuestions {
		return nil, fmt.Errorf("questions must contain 1 to %d entries", maxStructuredOutputQuestions)
	}
	if args.AutoResolutionMS != nil && (*args.AutoResolutionMS < 60_000 || *args.AutoResolutionMS > 240_000) {
		return nil, fmt.Errorf("autoResolutionMs must be between 60000 and 240000")
	}

	seen := make(map[string]struct{}, len(args.Questions))
	for index := range args.Questions {
		question := &args.Questions[index]
		question.ID = strings.TrimSpace(question.ID)
		question.Header = strings.TrimSpace(question.Header)
		question.Question = strings.TrimSpace(question.Question)
		if question.ID == "" || question.Header == "" || question.Question == "" {
			return nil, fmt.Errorf("question id, header, and question are required")
		}
		if _, exists := seen[question.ID]; exists {
			return nil, fmt.Errorf("duplicate question id %q", question.ID)
		}
		seen[question.ID] = struct{}{}
		if len(question.Options) > maxStructuredOutputQuestionOptions {
			return nil, fmt.Errorf("question %q has more than %d options", question.ID, maxStructuredOutputQuestionOptions)
		}
		for optionIndex := range question.Options {
			option := &question.Options[optionIndex]
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				return nil, fmt.Errorf("question %q contains an option without a label", question.ID)
			}
		}
	}
	return args, nil
}

func decodeResourceLinkRecord(payload json.RawMessage) (any, error) {
	var link activity.ResourceLink
	if err := json.Unmarshal(payload, &link); err != nil {
		return nil, err
	}
	link.Type = strings.TrimSpace(link.Type)
	link.Name = strings.TrimSpace(link.Name)
	link.Title = strings.TrimSpace(link.Title)
	link.URI = strings.TrimSpace(link.URI)
	link.Description = strings.TrimSpace(link.Description)
	link.MIMEType = strings.TrimSpace(link.MIMEType)
	if link.Type != structuredOutputResourceLink {
		return nil, fmt.Errorf("type must be %q", structuredOutputResourceLink)
	}
	if link.Name == "" || link.URI == "" {
		return nil, fmt.Errorf("name and uri are required")
	}
	if strings.ContainsAny(link.URI, "<>\r\n\t ") {
		return nil, fmt.Errorf("uri contains unsafe characters")
	}
	parsed, err := url.Parse(link.URI)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("uri must be an absolute HTTP(S) URL")
	}
	return link, nil
}

func structuredOutputArtifactEmpty(artifact activity.StructuredOutputArtifact) bool {
	return artifact.RequestUserInput == nil && len(artifact.ResourceLinks) == 0
}
