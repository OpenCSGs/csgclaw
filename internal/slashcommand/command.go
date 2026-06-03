package slashcommand

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const ElementName = "slash-command"

var commandNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var skillSlugPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

const (
	UseSkillCommandName = "use-skill"
)

type Command struct {
	Name string
	Arg  string
	Body string
}

func Parse(content string) (Command, bool, error) {
	text := strings.TrimSpace(content)
	if !looksLikeSlashCommand(text) {
		return Command{}, false, nil
	}

	decoder := xml.NewDecoder(strings.NewReader(text))
	token, err := decoder.Token()
	if err != nil {
		return Command{}, false, err
	}
	start, ok := token.(xml.StartElement)
	if !ok || start.Name.Space != "" || start.Name.Local != ElementName {
		return Command{}, false, nil
	}

	cmd, err := commandFromStart(start)
	if err != nil {
		return Command{}, false, err
	}

	var elementBody strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return Command{}, false, err
		}
		switch t := token.(type) {
		case xml.CharData:
			elementBody.Write([]byte(t))
		case xml.EndElement:
			if t.Name.Space != "" || t.Name.Local != ElementName {
				return Command{}, false, fmt.Errorf("unexpected closing tag %q", t.Name.Local)
			}
			if strings.TrimSpace(elementBody.String()) != "" {
				return Command{}, false, fmt.Errorf("slash command body must be empty")
			}
			end := int(decoder.InputOffset())
			if end < 0 || end > len(text) {
				return Command{}, false, fmt.Errorf("invalid token end offset")
			}
			cmd.Body = strings.TrimSpace(text[end:])
			if err := validate(cmd); err != nil {
				return Command{}, false, err
			}
			return cmd, true, nil
		case xml.StartElement:
			return Command{}, false, fmt.Errorf("slash command body must not contain child elements")
		case xml.Comment:
			return Command{}, false, fmt.Errorf("slash command body must be plain text")
		case xml.ProcInst, xml.Directive:
			return Command{}, false, fmt.Errorf("invalid slash command body token")
		default:
			return Command{}, false, fmt.Errorf("unsupported token in slash command body")
		}
	}
}

func Normalize(content string) (string, bool, error) {
	cmd, ok, err := Parse(content)
	if err != nil || !ok {
		return "", ok, err
	}
	rendered, err := Render(cmd)
	if err != nil {
		return "", false, err
	}
	return rendered, true, nil
}

func ParseFeishuShorthand(content string) (Command, bool, error) {
	text := strings.TrimSpace(content)
	if !strings.HasPrefix(text, "/") || strings.HasPrefix(text, "//") {
		return Command{}, false, nil
	}
	slug, body := splitSlashCommand(strings.TrimPrefix(text, "/"))
	if !validSkillSlug(slug) {
		return Command{}, false, nil
	}
	return Command{
		Name: UseSkillCommandName,
		Arg:  slug,
		Body: strings.TrimSpace(body),
	}, true, nil
}

func NormalizeFeishuInput(content string) (string, bool, error) {
	cmd, ok, err := ParseFeishuShorthand(content)
	if err != nil || !ok {
		return "", ok, err
	}
	rendered, err := Render(cmd)
	if err != nil {
		return "", false, err
	}
	return rendered, true, nil
}

func RenderFeishuFallback(content string) string {
	cmd, ok, err := Parse(content)
	if err != nil || !ok || strings.TrimSpace(cmd.Name) != UseSkillCommandName || !validSkillSlug(cmd.Arg) {
		return content
	}
	if body := strings.TrimSpace(cmd.Body); body != "" {
		return "/" + strings.TrimSpace(cmd.Arg) + " " + body
	}
	return "/" + strings.TrimSpace(cmd.Arg)
}

func Render(cmd Command) (string, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.Arg = strings.TrimSpace(cmd.Arg)
	cmd.Body = strings.TrimSpace(cmd.Body)
	if err := validate(cmd); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("<")
	b.WriteString(ElementName)
	b.WriteString(` name="`)
	b.WriteString(escapeXML(cmd.Name))
	b.WriteString(`"`)
	if cmd.Arg != "" {
		b.WriteString(` arg="`)
		b.WriteString(escapeXML(cmd.Arg))
		b.WriteString(`"`)
	}
	b.WriteString("></")
	b.WriteString(ElementName)
	b.WriteString(">")
	if cmd.Body != "" {
		b.WriteString(" ")
		b.WriteString(cmd.Body)
	}
	return b.String(), nil
}

func looksLikeSlashCommand(text string) bool {
	if !strings.HasPrefix(text, "<"+ElementName) {
		return false
	}
	if len(text) == len("<"+ElementName) {
		return true
	}
	next := text[len("<"+ElementName)]
	return next == ' ' || next == '	' || next == '\n' || next == '\r' || next == '>' || next == '/'
}

func splitSlashCommand(rest string) (string, string) {
	rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	for idx, r := range rest {
		if unicode.IsSpace(r) {
			return rest[:idx], rest[idx:]
		}
	}
	return rest, ""
}

func commandFromStart(start xml.StartElement) (Command, error) {
	cmd := Command{}
	seen := map[string]struct{}{}
	for _, attr := range start.Attr {
		if attr.Name.Space != "" {
			return Command{}, fmt.Errorf("slash command attributes must not use namespaces")
		}
		name := attr.Name.Local
		if _, ok := seen[name]; ok {
			return Command{}, fmt.Errorf("duplicate slash command attribute %q", name)
		}
		seen[name] = struct{}{}
		switch name {
		case "name":
			cmd.Name = strings.TrimSpace(attr.Value)
		case "arg":
			cmd.Arg = strings.TrimSpace(attr.Value)
		default:
			return Command{}, fmt.Errorf("unsupported slash command attribute %q", name)
		}
	}
	return cmd, nil
}

func validate(cmd Command) error {
	name := strings.TrimSpace(cmd.Name)
	if !commandNamePattern.MatchString(name) {
		return fmt.Errorf("invalid slash command name %q", cmd.Name)
	}
	arg := strings.TrimSpace(cmd.Arg)
	if len(arg) > 256 {
		return fmt.Errorf("slash command arg exceeds 256 bytes")
	}
	if strings.ContainsAny(arg, "\r\n\t") {
		return fmt.Errorf("slash command arg must be a single token")
	}
	return nil
}

func validSkillSlug(slug string) bool {
	slug = strings.TrimSpace(slug)
	return slug != "" && slug != "." && slug != ".." && !strings.ContainsAny(slug, `/\`) && skillSlugPattern.MatchString(slug)
}

func escapeXML(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}
