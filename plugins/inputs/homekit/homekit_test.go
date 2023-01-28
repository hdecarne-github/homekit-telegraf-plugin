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

func TestTempParsing(t *testing.T) {
	plugin := NewHomeKit()

	temperature1c, err := plugin.parseTempValue("1 °C")
	require.Nil(t, err)
	require.Equal(t, 1.0, temperature1c)

	temperature2_1c, err := plugin.parseTempValue("2,1 °C")
	require.Nil(t, err)
	require.Equal(t, 2.1, temperature2_1c)

	temperature3c, err := plugin.parseTempValue("-16.1111 °F")
	require.Nil(t, err)
	require.Equal(t, 3.0, temperature3c)

	_, err = plugin.parseTempValue("a.b °C")
	require.NotNil(t, err)

	_, err = plugin.parseTempValue("1.2 °Q")
	require.NotNil(t, err)
}

func TestLightLevelParsing(t *testing.T) {
	plugin := NewHomeKit()

	level1lx, err := plugin.parseLightLevelValue("1 lx")
	require.Nil(t, err)
	require.Equal(t, 1.0, level1lx)

	level2_34lx, err := plugin.parseLightLevelValue("2,341 lx")
	require.Nil(t, err)
	require.Equal(t, 2.34, level2_34lx)

	_, err = plugin.parseLightLevelValue("a.b lx")
	require.NotNil(t, err)

	_, err = plugin.parseLightLevelValue("1.2 lmn")
	require.NotNil(t, err)
}
