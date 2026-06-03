package slashcommand

import "testing"

func TestParseCanonicalSlashCommandPrefix(t *testing.T) {
	cmd, ok, err := Parse(`<slash-command name="use-skill" arg="skill-creator"></slash-command> create a review skill`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !ok {
		t.Fatal("Parse() ok = false, want true")
	}
	if cmd.Name != "use-skill" || cmd.Arg != "skill-creator" || cmd.Body != "create a review skill" {
		t.Fatalf("command = %+v, want use-skill skill-creator prompt body", cmd)
	}
}

func TestParseCanonicalSlashCommandSelfClosingPrefix(t *testing.T) {
	cmd, ok, err := Parse(`<slash-command name="use-skill" arg="skill-creator"/> create a review skill`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !ok {
		t.Fatal("Parse() ok = false, want true")
	}
	if cmd.Body != "create a review skill" {
		t.Fatalf("Body = %q, want trailing prompt", cmd.Body)
	}
}

func TestNormalizeCanonicalSlashCommandPrefix(t *testing.T) {
	got, ok, err := Normalize(`  <slash-command arg="skill-creator" name="use-skill"/>  create & review <safely>  `)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if !ok {
		t.Fatal("Normalize() ok = false, want true")
	}
	want := `<slash-command name="use-skill" arg="skill-creator"></slash-command> create & review <safely>`
	if got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestRenderKeepsUserPromptOutsideCommandElement(t *testing.T) {
	got, err := Render(Command{Name: "use-skill", Arg: "skill-creator", Body: `1 < 2 & "quote"`})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	want := `<slash-command name="use-skill" arg="skill-creator"></slash-command> 1 < 2 & "quote"`
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestParseRejectsLegacySlashText(t *testing.T) {
	if _, ok, err := Parse(`/skill-creator create a review skill`); ok || err != nil {
		t.Fatalf("Parse(legacy slash) = ok %v err %v, want ok false err nil", ok, err)
	}
}

func TestNormalizeFeishuInputConvertsSlashSkillShorthand(t *testing.T) {
	got, ok, err := NormalizeFeishuInput(`/skill-creator create a review skill`)
	if err != nil {
		t.Fatalf("NormalizeFeishuInput() error = %v", err)
	}
	if !ok {
		t.Fatal("NormalizeFeishuInput() ok = false, want true")
	}
	want := `<slash-command name="use-skill" arg="skill-creator"></slash-command> create a review skill`
	if got != want {
		t.Fatalf("NormalizeFeishuInput() = %q, want %q", got, want)
	}
}

func TestParseFeishuShorthandFallsBackOnInvalidSlug(t *testing.T) {
	cmd, ok, err := ParseFeishuShorthand(`/foo/bar create a review skill`)
	if err != nil {
		t.Fatalf("ParseFeishuShorthand() error = %v", err)
	}
	if ok {
		t.Fatalf("ParseFeishuShorthand() ok = true, want false (fallback to plain text)")
	}
	if cmd != (Command{}) {
		t.Fatalf("ParseFeishuShorthand() cmd = %+v, want empty", cmd)
	}
}

func TestNormalizeRejectsMalformedSlashCommandPrefix(t *testing.T) {
	got, ok, err := Normalize(`<slash-command name="use-skill"`)
	if err == nil {
		t.Fatal("Normalize() err = nil, want malformed command error")
	}
	if ok {
		t.Fatal("Normalize() ok = true, want false")
	}
	if got != "" {
		t.Fatalf("Normalize() = %q, want empty", got)
	}
}

func TestRenderFeishuFallbackConvertsCanonicalUseSkillToSlashText(t *testing.T) {
	got := RenderFeishuFallback(`<slash-command name="use-skill" arg="skill-creator"></slash-command> create a review skill`)
	want := `/skill-creator create a review skill`
	if got != want {
		t.Fatalf("RenderFeishuFallback() = %q, want %q", got, want)
	}
}

func TestParseRejectsNestedSlashCommandElement(t *testing.T) {
	_, ok, err := Parse(`<slash-command name="use-skill" arg="skill-creator"><b>bad</b></slash-command> prompt`)
	if err != nil || ok {
		t.Fatalf("Parse(nested) = ok=%v err=%v, want plain-text fallback", ok, err)
	}
}

func TestParseRejectsMalformedSlashCommand(t *testing.T) {
	for _, input := range []string{
		`<slash-command name=""></slash-command> body`,
		`<slash-command/> body`,
	} {
		_, ok, err := Parse(input)
		if err == nil || ok {
			t.Fatalf("Parse(%q) = ok=%v err=%v, want malformed command error", input, ok, err)
		}
	}
}

func TestParseRejectsDuplicateSlashCommandAttribute(t *testing.T) {
	_, ok, err := Parse(`<slash-command name="use-skill" name="use-skill" arg="skill-creator"></slash-command>`)
	if err == nil || ok {
		t.Fatalf("Parse(duplicate-attr) = ok=%v err=%v, want malformed command error", ok, err)
	}
}

func TestParseRejectsPromptInsideSlashCommandElement(t *testing.T) {
	_, ok, err := Parse(`<slash-command name="use-skill" arg="skill-creator">prompt</slash-command>`)
	if err != nil || ok {
		t.Fatalf("Parse(inline prompt) ok=%v err=%v, want plain-text fallback", ok, err)
	}
}
