package config

import "testing"

func TestDefaultSandboxProviderUsesDockerEvenWhenBundledBoxLiteIsPresent(t *testing.T) {
	if got, want := defaultSandboxProvider(), DockerProvider; got != want {
		t.Fatalf("defaultSandboxProvider() = %q, want %q", got, want)
	}
}

func TestDefaultSandboxProviderFallsBackToDockerWhenBundledBoxLiteIsAbsent(t *testing.T) {
	if got, want := defaultSandboxProvider(), DockerProvider; got != want {
		t.Fatalf("defaultSandboxProvider() = %q, want %q", got, want)
	}
}
