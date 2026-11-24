package codex

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Change only the literal false values belonging to this skill. Re-encoding
// config.toml would discard comments and rewrite unrelated user settings.
func enableSkillConfigEntries(content, target string) (string, error) {
	type entry struct {
		path     string
		disabled unstable.Range
	}
	var edits []unstable.Range
	commit := func(e entry) {
		if e.disabled.Length != 0 && filepath.Clean(e.path) == target {
			edits = append(edits, e.disabled)
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
			for it := v.Children(); it.Next(); {
				if it.Node().Kind != unstable.InlineTable {
					continue
				}
				var e entry
				for fields := it.Node().Children(); fields.Next(); {
					if fields.Node().Kind == unstable.KeyValue {
						field(&e, fields.Node())
					}
				}
				commit(e)
			}
		}
	}
	var p unstable.Parser
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
		return "", err
	}
	if active != nil {
		commit(*active)
	}
	if len(edits) == 0 {
		return content, nil
	}
	var out strings.Builder
	start := 0
	for _, edit := range edits {
		offset := int(edit.Offset)
		out.WriteString(content[start:offset])
		out.WriteString("true")
		start = offset + int(edit.Length)
	}
	out.WriteString(content[start:])
	return out.String(), nil
}
