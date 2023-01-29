// homekit_test.go
//
// # Copyright (C) 2023 Holger de Carne
//
// This software may be modified and distributed under the terms
// of the MIT license.  See the LICENSE file for details.
package homekit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	plugin := NewHomeKit()
	require.NotNil(t, plugin)
}

func TestSampleConfig(t *testing.T) {
	plugin := NewHomeKit()
	sampleConfig := plugin.SampleConfig()
	require.NotNil(t, sampleConfig)
}

func TestDescription(t *testing.T) {
	plugin := NewHomeKit()
	description := plugin.Description()
	require.NotNil(t, description)
}
