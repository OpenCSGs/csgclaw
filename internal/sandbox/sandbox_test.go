package sandbox

import (
	"context"
	"errors"
	"testing"
)

type testProvider struct{}

func (testProvider) Name() string {
	return "test"
}

func (testProvider) Open(context.Context, string) (Runtime, error) {
	return testRuntime{}, nil
}

func (testProvider) ListImages(context.Context, string) ([]string, error) {
	return nil, nil
}

type testRuntime struct{}

func (testRuntime) Create(context.Context, CreateSpec) (Instance, error) {
	return testInstance{}, nil
}

func (testRuntime) Get(context.Context, string) (Instance, error) {
	return testInstance{}, nil
}

func (testRuntime) Remove(context.Context, string, RemoveOptions) error {
	return nil
}

func (testRuntime) Close() error {
	return nil
}

type testInstance struct{}

func (testInstance) Start(context.Context) error {
	return nil
}

func (testInstance) Stop(context.Context, StopOptions) error {
	return nil
}

func (testInstance) Info(context.Context) (Info, error) {
	return Info{}, nil
}

func (testInstance) Run(context.Context, CommandSpec) (CommandResult, error) {
	return CommandResult{}, nil
}

func (testInstance) Close() error {
	return nil
}

func TestInterfacesCompile(t *testing.T) {
	var _ Provider = testProvider{}
	var _ Runtime = testRuntime{}
	var _ Instance = testInstance{}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(ErrNotFound) {
		t.Fatal("ErrNotFound should be recognized")
	}
	if !IsNotFound(errors.Join(errors.New("adapter lookup failed"), ErrNotFound)) {
		t.Fatal("wrapped ErrNotFound should be recognized")
	}
	if IsNotFound(nil) {
		t.Fatal("nil should not be recognized as not found")
	}
	if IsNotFound(errors.New("other error")) {
		t.Fatal("unrelated error should not be recognized as not found")
	}
}
