// homekit_test.go
//
// # Copyright (C) 2023 Holger de Carne
//
// This software may be modified and distributed under the terms
// of the MIT license.  See the LICENSE file for details.
package homekit

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/influxdata/telegraf/testutil"
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

func TestRunSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	plugin := NewHomeKit()
	plugin.Address = address
	plugin.HAPStorePath = "../../../build/.hap"
	plugin.MonitorAccessoryName = "TestMonitor"
	plugin.Log = createDummyLogger()
	plugin.Debug = true

	acc := &testutil.Accumulator{}

	defer plugin.Stop()
	require.NoError(t, plugin.Start(acc))
	require.NoError(t, plugin.Gather(acc))

	statusCode := putJson(t, address, `{
		"Name": "Yes"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_state",
		map[string]interface{}{
			"active":  true,
			"percent": 100},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "undefined",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room": "No"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_state",
		map[string]interface{}{
			"active":  false,
			"percent": 0},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room_Light": "Ja"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_state",
		map[string]interface{}{
			"active":  true,
			"percent": 100},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "Light"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room": "Nein"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_state",
		map[string]interface{}{
			"active":  false,
			"percent": 0},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room": "12.3 °C"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_temperature",
		map[string]interface{}{
			"celsius":    12.3,
			"fahrenheit": 54.14},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room": "54.14 °F"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_temperature",
		map[string]interface{}{
			"celsius":    12.3,
			"fahrenheit": 54.14},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
				"Name_Room": "12,3 °C"
			}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_temperature",
		map[string]interface{}{
			"celsius":    12.3,
			"fahrenheit": 54.14},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
				"Name_Room": "54,14 °F"
			}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_temperature",
		map[string]interface{}{
			"celsius":    12.3,
			"fahrenheit": 54.14},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room": "10 lx"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_light_level",
		map[string]interface{}{
			"lux": 10.0},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})

	acc.ClearMetrics()
	statusCode = putJson(t, address, `{
		"Name_Room": "360°"
	}`)
	require.Equal(t, http.StatusOK, statusCode)
	acc.AssertContainsTaggedFields(t, "homekit_light_hue",
		map[string]interface{}{
			"hue": 360},
		map[string]string{
			"homekit_monitor":        "TestMonitor",
			"homekit_name":           "Name",
			"homekit_room":           "Room",
			"homekit_characteristic": "generic"})
}

func putJson(t *testing.T, address string, json string) int {
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("http://%s/monitor", address), strings.NewReader(json))
	require.NoError(t, err)
	req.Header.Add("Content-type", "application/json")
	rsp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return rsp.StatusCode
}

func TestRunFailures(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	plugin := NewHomeKit()
	plugin.Address = address
	plugin.HAPStorePath = "../../../build/.hap"
	plugin.MonitorAccessoryName = "TestMonitor"
	plugin.Log = createDummyLogger()
	plugin.Debug = true

	acc := &testutil.Accumulator{}

	defer plugin.Stop()
	require.NoError(t, plugin.Start(acc))
	require.NoError(t, plugin.Gather(acc))

	rsp, err := http.Get(fmt.Sprintf("http://%s/monitor", address))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rsp.StatusCode)

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("http://%s/monitor", address), strings.NewReader("<xml></xml>"))
	req.Header.Add("Content-type", "application/xml")
	require.NoError(t, err)
	rsp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rsp.StatusCode)
}

func createDummyLogger() *dummyLogger {
	log.SetOutput(os.Stderr)
	return &dummyLogger{}
}

type dummyLogger struct{}

func (l *dummyLogger) Errorf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *dummyLogger) Error(args ...interface{}) {
	log.Print(args...)
}

func (l *dummyLogger) Debugf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *dummyLogger) Debug(args ...interface{}) {
	log.Print(args...)
}

func (l *dummyLogger) Warnf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *dummyLogger) Warn(args ...interface{}) {
	log.Print(args...)
}

func (l *dummyLogger) Infof(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *dummyLogger) Info(args ...interface{}) {
	log.Print(args...)
}
