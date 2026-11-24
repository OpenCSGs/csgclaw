package codex

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

type skillConfigEdit struct {
	start, end int
	text       string
}

// Edit literals and managed entries without re-encoding comments and unrelated settings.
func editSkillConfigEntries(content, target string, paths []string) (string, bool, error) {
	type entry struct {
		path     string
		disabled unstable.Range
	}
	var edits []skillConfigEdit
	var hasInline bool
	commit := func(e entry) {
		if target != "" && e.disabled.Length != 0 && filepath.Clean(e.path) == target {
			edits = append(edits, skillConfigEdit{int(e.disabled.Offset), int(e.disabled.Offset + e.disabled.Length), "true"})
		}
	}
	keys := func(n *unstable.Node) []string {
		var out []string
		it := n.Key()
		for it.Next() {
			out = append(out, string(it.Node().Data))
		}
		return out
	}
	field := func(e *entry, n *unstable.Node) {
		k := keys(n)
		if len(k) != 1 {
			return
		}
		v := n.Value()
		switch k[0] {
		case "path":
			if v.Kind == unstable.String {
				e.path = string(v.Data)
			}
		case "enabled":
			if v.Kind == unstable.Bool && string(v.Data) == "false" {
				e.disabled = v.Raw
			}
		}
	}
	configKey := []string{"skills", "config"}
	var inline func([]string, *unstable.Node)
	inline = func(prefix []string, n *unstable.Node) {
		k := append(slices.Clone(prefix), keys(n)...)
		v := n.Value()
		switch {
		case slices.Equal(k, []string{"skills"}) && v.Kind == unstable.InlineTable:
			for it := v.Children(); it.Next(); {
				if it.Node().Kind == unstable.KeyValue {
					inline(k, it.Node())
				}
			}
		case slices.Equal(k, configKey) && v.Kind == unstable.Array:
			hasInline = true
			slot := inlineSkillSlot(content, n)
			alreadyDisabled := make(map[string]bool)
			for it := v.Children(); it.Next(); {
				offset := int(it.Node().Raw.Offset)
				if it.Node().Kind != unstable.InlineTable || (offset >= slot.start && offset < slot.end) {
					continue
				}
				var e entry
				for fields := it.Node().Children(); fields.Next(); {
					if fields.Node().Kind == unstable.KeyValue {
						field(&e, fields.Node())
					}
				}
				commit(e)
				if e.disabled.Length != 0 {
					alreadyDisabled[filepath.Clean(e.path)] = true
				}
			}
			var entries []string
			for _, path := range paths {
				if !alreadyDisabled[path] {
					entries = append(entries, "{ path = "+strconv.Quote(path)+", enabled = false }")
				}
			}
			if len(entries) > 0 {
				prefix := strings.TrimRight(content[:slot.start], " \t\r\n")
				separator := ""
				if !strings.HasSuffix(prefix, "[") && !strings.HasSuffix(prefix, ",") {
					separator = ","
				}
				slot.text = "\n" + inlineSkillBegin + "\n" + separator + strings.Join(entries, ",\n") + "\n" + inlineSkillEnd + "\n"
			}
			edits = append(edits, slot)
		}
	}
	var p unstable.Parser
	p.KeepComments = true
	p.Reset([]byte(content))
	var table []string
	var active *entry
	for p.NextExpression() {
		n := p.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			if active != nil {
				commit(*active)
				active = nil
			}
			table = keys(n)
			if n.Kind == unstable.ArrayTable && slices.Equal(table, configKey) {
				active = &entry{}
			}
		case unstable.KeyValue:
			if active != nil {
				field(active, n)
			} else {
				inline(table, n)
			}
		}
	}
	if err := p.Error(); err != nil {
		return "", false, err
	}
	if active != nil {
		commit(*active)
	}
	if len(edits) == 0 {
		return content, hasInline, nil
	}
	slices.SortStableFunc(edits, func(a, b skillConfigEdit) int { return a.start - b.start })
	var out strings.Builder
	start := 0
	for _, edit := range edits {
		out.WriteString(content[start:edit.start])
		out.WriteString(edit.text)
		start = edit.end
	}
	out.WriteString(content[start:])
	return out.String(), hasInline, nil
}

// Keep the generated portion inside the original array, ahead of any trailing
// comments. Its separators belong to the block so removing it restores user text.
const inlineSkillBegin = "# BEGIN CSGCLAW INLINE SKILL STATES"
const inlineSkillEnd = "# END CSGCLAW INLINE SKILL STATES"

func inlineSkillSlot(content string, kv *unstable.Node) skillConfigEdit {
	closeBracket := int(kv.Raw.Offset+kv.Raw.Length) - 1
	at, begin := closeBracket, -1
	for it := kv.Value().Children(); it.Next(); {
		n := it.Node()
		if n.Kind != unstable.Comment {
			at = closeBracket
			continue
		}
		i := int(n.Raw.Offset)
		if at == closeBracket {
			at = i
		}
		switch string(n.Data) {
		case inlineSkillBegin:
			if i > 0 && content[i-1] == '\n' {
				begin = i - 1
				if begin > 0 && content[begin-1] == '\r' {
					begin--
				}
			}
		case inlineSkillEnd:
			end := i + int(n.Raw.Length)
			if end < len(content) && content[end] == '\r' {
				end++
			}
			if begin >= 0 && end < len(content) && content[end] == '\n' {
				return skillConfigEdit{start: begin, end: end + 1}
			}
		}
	}
	return skillConfigEdit{start: at, end: at}
}
