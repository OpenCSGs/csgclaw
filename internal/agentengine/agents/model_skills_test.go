package agents

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCreateAgentSpecJSONPreservesSkillSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		skills []string
	}{
		{name: "omitted"},
		{name: "empty", skills: []string{}},
		{name: "selected", skills: []string{"reviewer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(CreateAgentSpec{Name: "reviewer", Skills: tc.skills})
			if err != nil {
				t.Fatal(err)
			}
			var decoded CreateAgentSpec
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.Skills, tc.skills) {
				t.Fatalf("round-trip skills = %#v, want %#v", decoded.Skills, tc.skills)
			}
		})
	}
}
