package upgrade

import (
	"reflect"
	"testing"
)

func TestCommandArgsWithConfig(t *testing.T) {
	got := commandArgsWithConfig("/tmp/config.toml", "_restart")
	want := []string{"--config", "/tmp/config.toml", "_restart"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandArgsWithConfig() = %#v, want %#v", got, want)
	}

	got = commandArgsWithConfig("", "serve", "-d")
	want = []string{"serve", "-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandArgsWithConfig() empty path = %#v, want %#v", got, want)
	}
}
