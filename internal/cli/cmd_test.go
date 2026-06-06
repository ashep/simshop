package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCmd(main *testing.T) {
	main.Run("has order and shops subcommands", func(t *testing.T) {
		root := NewRootCmd()
		names := map[string]bool{}
		for _, c := range root.Commands() {
			names[c.Name()] = true
		}
		assert.True(t, names["order"])
		assert.True(t, names["shops"])
	})

	main.Run("set-status rejects invalid status before any request", func(t *testing.T) {
		root := NewRootCmd()
		root.SetArgs([]string{"order", "set-status", "some-id", "bogus"})
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		err := root.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status")
	})
}
