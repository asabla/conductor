package executor

import (
	"context"
	"testing"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
)

type stubExecutor struct {
	name string
}

func (s stubExecutor) Execute(ctx context.Context, req *ExecutionRequest, reporter ResultReporter) (*ExecutionResult, error) {
	return &ExecutionResult{}, nil
}

func (s stubExecutor) Name() string {
	return s.name
}

func TestFactory(t *testing.T) {
	subprocess := stubExecutor{name: "subprocess"}
	container := stubExecutor{name: "container"}

	t.Run("returns container when available", func(t *testing.T) {
		exec := Factory(conductorv1.ExecutionType_EXECUTION_TYPE_CONTAINER, subprocess, container)
		if exec.Name() != "container" {
			t.Fatalf("expected container executor, got %s", exec.Name())
		}
	})

	t.Run("falls back to subprocess when container is nil", func(t *testing.T) {
		exec := Factory(conductorv1.ExecutionType_EXECUTION_TYPE_CONTAINER, subprocess, nil)
		if exec.Name() != "subprocess" {
			t.Fatalf("expected subprocess executor, got %s", exec.Name())
		}
	})

	t.Run("defaults to subprocess for unknown type", func(t *testing.T) {
		exec := Factory(conductorv1.ExecutionType_EXECUTION_TYPE_UNSPECIFIED, subprocess, container)
		if exec.Name() != "subprocess" {
			t.Fatalf("expected subprocess executor, got %s", exec.Name())
		}
	})
}
