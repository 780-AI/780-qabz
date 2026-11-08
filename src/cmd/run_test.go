package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnknownCommand(t *testing.T) {
	assert.Equal(t, 2, Run([]string{"nope"}))
}

func TestMigrateWithoutAction(t *testing.T) {
	assert.Equal(t, 2, Run([]string{"migrate"}))
}

func TestDevDBWithoutAction(t *testing.T) {
	assert.Equal(t, 2, Run([]string{"devdb"}))
}

// start-app without a usable --config panics: that proves each form reached start-app.
func TestStartAppDispatch(t *testing.T) {
	assert.Panics(t, func() { Run(nil) })
	assert.Panics(t, func() { Run([]string{"start-app"}) })
	assert.Panics(t, func() { Run([]string{"--config=/no/such/file.yml"}) })
}
