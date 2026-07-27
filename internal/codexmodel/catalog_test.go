package codexmodel

import (
	"reflect"
	"testing"
)

func TestMetadataAdvertisesTheCommonReasoningContract(t *testing.T) {
	metadata := Metadata(Profile{ModelID: "gpt-5", ReasoningEffort: "off"})
	if got, want := metadata["default_reasoning_level"], "none"; got != want {
		t.Fatalf("default_reasoning_level = %v, want %v", got, want)
	}
	levels := metadata["supported_reasoning_levels"].([]map[string]any)
	got := make([]string, 0, len(levels))
	for _, level := range levels {
		got = append(got, level["effort"].(string))
	}
	want := []string{"none", "minimal", "low", "medium", "high", "xhigh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("supported reasoning levels = %#v, want %#v", got, want)
	}
}

func TestMetadataLeavesDefaultReasoningUnsetForAuto(t *testing.T) {
	metadata := Metadata(Profile{ModelID: "gpt-5", ReasoningEffort: "auto"})
	if got := metadata["default_reasoning_level"]; got != nil {
		t.Fatalf("default_reasoning_level = %v, want nil", got)
	}
}
