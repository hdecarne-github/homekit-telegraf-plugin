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

func TestRun(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	plugin := NewHomeKit()
	plugin.Address = address
	plugin.AuthorizationRequired = false
	plugin.Log = createDummyLogger()
	plugin.Debug = true

	acc := &testutil.Accumulator{}

	defer plugin.Stop()
	require.NoError(t, plugin.Start(acc))
	require.NoError(t, plugin.Gather(acc))

	rsp, err := putJson(address, `{
		"State accessory A_Room 1": "Yes",
		"State accessory B_Room 1": "No",
		"Light accessory A_Room 1_Light": "Yes",
		"Light accessory B_Room 1_Light": "No",
		"State accessory C_Room 1": "Ja",
		"State accessory D_Room 1": "Nein",
		"Temperature sensor accessory E_Room 2": "42,3 °C",
		"Temperature sensor accessory F_Room 2": "42,3 °F",
		"Light sensor accessory G_Room 3": "10 lx",
		"Light accessory G_Room 3": "360°"
	}`)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)
}

func putJson(address string, json string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("http://%s/monitor", address), strings.NewReader(json))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-type", "application/json")
	return http.DefaultClient.Do(req)
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
