package runtime

import (
	"slices"
	"strings"
	"testing"
)

func TestMergeExtensionEnvironmentUsesCaseInsensitiveKeys(t *testing.T) {
	projection := ExtensionProjection{
		Name:        "feishu-lark-cli",
		Digest:      "digest-1",
		Environment: map[string]string{"LARK_CHANNEL": "1"},
	}

	environment, digests, err := MergeExtensionEnvironment(
		[]string{"PATH=/usr/bin", "lark_channel=host"},
		nil,
		[]ExtensionProjection{projection},
	)
	if err != nil {
		t.Fatalf("MergeExtensionEnvironment() error = %v", err)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "lark_channel=") || !strings.Contains(joined, "LARK_CHANNEL=1") {
		t.Fatalf("environment = %q", joined)
	}
	if digests[projection.Name] != projection.Digest {
		t.Fatalf("digests = %#v", digests)
	}
}

func TestMergeExtensionEnvironmentRejectsCaseInsensitiveProfileConflict(t *testing.T) {
	_, _, err := MergeExtensionEnvironment(nil, map[string]string{"lark_channel": "profile"}, []ExtensionProjection{{
		Name:        "feishu-lark-cli",
		Environment: map[string]string{"LARK_CHANNEL": "1"},
	}})
	if err == nil || !strings.Contains(err.Error(), "conflicts with the Agent profile") {
		t.Fatalf("MergeExtensionEnvironment() error = %v", err)
	}
}

func TestMergeExtensionEnvironmentPreservesNonemptyProxyBypass(t *testing.T) {
	for _, tc := range []struct {
		name string
		base []string
		want string
	}{
		{
			name: "empty lowercase",
			base: []string{"NO_PROXY=fixture.invalid", "no_proxy="},
			want: "NO_PROXY=fixture.invalid",
		},
		{
			name: "empty uppercase",
			base: []string{"no_proxy=fixture.invalid", "NO_PROXY="},
			want: "no_proxy=fixture.invalid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environment, _, err := MergeExtensionEnvironment(tc.base, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(environment, []string{tc.want}) {
				t.Fatalf("environment = %q, want %q", environment, tc.want)
			}
		})
	}
}

func TestMergeExtensionEnvironmentProxyBypassOverride(t *testing.T) {
	environment, _, err := MergeExtensionEnvironment(
		[]string{"NO_PROXY=*", "no_proxy= "},
		nil,
		[]ExtensionProjection{{Name: "proxy", Environment: map[string]string{"NO_PROXY": ".extension.example"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(environment, []string{"NO_PROXY=.extension.example"}) {
		t.Fatalf("environment = %q, want the extension's exclusion list", environment)
	}
}

func TestExtensionInstructionsPreservesProjectionOrder(t *testing.T) {
	got := ExtensionInstructions([]ExtensionProjection{
		{Name: "first", Instructions: "first instructions"},
		{Name: "empty"},
		{Name: "second", Instructions: "second instructions"},
	})
	want := []string{"first instructions", "second instructions"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ExtensionInstructions() = %#v, want %#v", got, want)
	}
}
