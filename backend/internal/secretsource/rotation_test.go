package secretsource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"getarcane.dev/arcane/v3/internal/scheduler/flow/flowtest"
)

func TestSecretRotationWorkflowDefinition(t *testing.T) {
	s, _, _ := newSecretFixture(t)
	harness := flowtest.New(t, nil)
	module := New(Params{Service: s})
	require.NoError(t, module.RegisterWorkflows(harness.Engine))
	harness.Start(t)
	flowtest.AssertDefinitions(t, harness)
}
