// homekit.go
//
// Copyright (C) 2023 Holger de Carne
//
// This software may be modified and distributed under the terms
// of the MIT license.  See the LICENSE file for details.

package homekit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	dnssdlog "github.com/brutella/dnssd/log"
	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	haplog "github.com/brutella/hap/log"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

var serialNumber = "1"
var firmware = "0.0.0"
var manufacturer = "https://hdecarne-github.github.io/homekit-telegraf-plugin/"
var model = "homekit-telegraf-plugin"

type HomeKit struct {
	Address               string   `toml:"address"`
	MonitorPath           string   `toml:"monitor_path"`
	AuthorizationRequired bool     `toml:"authorization_required"`
	HAPStorePath          string   `toml:"hap_store_path"`
	MonitorAccessoryName  string   `toml:"monitor_accessory_name"`
	MonitorAccessoryPin   string   `toml:"monitor_accessory_pin"`
	ActiveStateValues     []string `toml:"active_state_values"`
	Debug                 bool     `toml:"debug"`

	Log telegraf.Logger

	acc telegraf.Accumulator

	accessory     *accessory.Switch
	server        *hap.Server
	serverCtx     context.Context
	stopServer    context.CancelFunc
	serverStopped sync.WaitGroup
}

func NewHomeKit() *HomeKit {
	return &HomeKit{
		Address:               ":8001",
		MonitorPath:           "/monitor",
		AuthorizationRequired: true,
		HAPStorePath:          ".hap",
		MonitorAccessoryName:  "Monitor",
		MonitorAccessoryPin:   "00102003",
		ActiveStateValues:     []string{"Yes"}}
}

func (plugin *HomeKit) SampleConfig() string {
	return `
  ## The address (host:port) to run the HAP server on
  address = ":8001"
  ## The path to receive monitor requests on
  # monitor_path = "/monitor"
  ## Only allow authorized clients to send monitor requests
  # authorization_required = true
  ## The directory path to create for storing the HAP state (e.g. paring state)
  # hap_store_path = ".hap"
  ## The name of the monitor accessory to use for triggering home automation
  # monitor_accessory_name = "Monitor"
  ## The pin to use for pairing the monitor accessory
  # monitor_accessory_pin = 00102003
  ## Active state values
  # active_state_values = ["Yes"]
  ## Enable debug output
  # debug = false
`
}

func (plugin *HomeKit) Description() string {
	return "Monitor HomeKit stats (reported via Home automation)"
}

func (plugin *HomeKit) Gather(acc telegraf.Accumulator) error {
	if plugin.Debug {
		plugin.Log.Infof("Triggering monitor accessory: %s", plugin.MonitorAccessoryName)
	}
	plugin.accessory.Switch.On.SetValue(true)
	time.Sleep(100 * time.Millisecond)
	plugin.accessory.Switch.On.SetValue(false)
	return nil
}

func (plugin *HomeKit) Start(acc telegraf.Accumulator) error {
	plugin.acc = acc
	if !plugin.Debug {
		haplog.Info.Disable()
		dnssdlog.Info.Disable()
	}
	plugin.Log.Infof("Setting up monitor accessory: %s", plugin.MonitorAccessoryName)
	haplog.Info.Disable()
	plugin.accessory = accessory.NewSwitch(accessory.Info{
		Name:         plugin.MonitorAccessoryName,
		SerialNumber: serialNumber,
		Manufacturer: manufacturer,
		Firmware:     firmware,
		Model:        model,
	})
	plugin.Log.Infof("Starting HAP server: http://%s%s", plugin.Address, plugin.MonitorPath)
	server, err := hap.NewServer(hap.NewFsStore(plugin.HAPStorePath), plugin.accessory.A)
	if err != nil {
		plugin.Log.Errorf("Failed to start HAP server (%v)", err)
		return err
	}
	server.Addr = plugin.Address
	server.Pin = plugin.MonitorAccessoryPin
	server.ServeMux().HandleFunc(plugin.MonitorPath, plugin.monitor)
	serverCtx, stopServer := context.WithCancel(context.Background())
	plugin.serverStopped.Add(1)
	go func() {
		defer plugin.serverStopped.Done()
		_ = server.ListenAndServe(serverCtx)
	}()
	plugin.server = server
	plugin.serverCtx = serverCtx
	plugin.stopServer = stopServer
	return nil
}

func (plugin *HomeKit) Stop() {
	plugin.Log.Infof("Stopping HAP server: http://%s%s", plugin.Address, plugin.MonitorPath)
	if plugin.stopServer != nil {
		plugin.stopServer()
	}
	plugin.serverStopped.Wait()
}

func (plugin *HomeKit) monitor(res http.ResponseWriter, req *http.Request) {
	select {
	case <-plugin.serverCtx.Done():
		res.WriteHeader(http.StatusGone)
		return
	default:
	}
	if plugin.Debug {
		plugin.Log.Infof("Handling monitor request: %s", req.RemoteAddr)
	}
	if plugin.AuthorizationRequired && !plugin.server.IsAuthorized(req) {
		plugin.Log.Warnf("Client not authorized: %s", req.RemoteAddr)
		res.WriteHeader(http.StatusForbidden)
		return
	}
	if req.URL.Path != plugin.MonitorPath {
		plugin.Log.Warnf("Invalid path: %s", req.URL.Path)
		http.NotFound(res, req)
		return
	}
	if req.Method != http.MethodPut {
		plugin.Log.Warnf("Invalid method: %s", req.Method)
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	contentType := req.Header.Get("Content-type")
	if contentType != "application/json" {
		plugin.Log.Warnf("Invalid content type: %s", contentType)
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	defer req.Body.Close()
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		plugin.Log.Warnf("Inaccessible request body: %v", err)
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	var data map[string]string
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		plugin.Log.Warnf("Invalid request body: %v", err)
		res.WriteHeader(http.StatusBadRequest)
		return
	}
	plugin.processData(data)
	res.Write([]byte("Ok"))
}

func (plugin *HomeKit) processData(data map[string]string) {
	for key, value := range data {
		keyParts := strings.SplitN(key, "_", 3)
		if len(keyParts) != 3 {
			plugin.Log.Warnf("Invalid data key: %s", key)
			continue
		}
		dataType := keyParts[0]
		dataRoom := keyParts[1]
		dataName := keyParts[2]
		switch dataType {
		case "TEMP":
			plugin.processTempData(dataType, dataRoom, dataName, value)
		case "LIGHT":
			plugin.processLightData(dataType, dataRoom, dataName, value)
		case "LIGHTLEVEL":
			plugin.processLightLevelData(dataType, dataRoom, dataName, value)
		case "STATE":
			plugin.processStateData(dataType, dataRoom, dataName, value)
		default:
			plugin.Log.Warnf("Unrecognized data entry: %s = %s", key, value)
		}
	}
}

func (plugin *HomeKit) processTempData(dataType string, dataRoom string, dataName string, value string) {
	temperature, err := plugin.parseTempValue(value)
	if err != nil {
		plugin.Log.Warnf("Failed to process temperature data: %s (cause: %v)", value, err)
		return
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_room"] = dataRoom
	tags["homekit_accessory"] = dataName
	fields := make(map[string]interface{})
	fields["temperature"] = temperature
	plugin.acc.AddCounter("homekit_temperature", fields, tags)
}

func (plugin *HomeKit) parseTempValue(value string) (float64, error) {
	parsed := 0.0
	cSuffix := " °C"
	fSuffix := " °F"
	suffix := cSuffix
	conversion := func(c float64) float64 { return c }
	if strings.HasSuffix(value, fSuffix) {
		suffix = fSuffix
		conversion = func(c float64) float64 { return (c * 1.8) + 32.0 }
	} else if !strings.HasSuffix(value, cSuffix) {
		return parsed, fmt.Errorf("unrecognized temperature value: %s", value)
	}
	rawValue := strings.TrimSuffix(value, suffix)
	rawValue = strings.ReplaceAll(rawValue, ",", ".")
	parsed, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return parsed, fmt.Errorf("failed to parse temperature value: %s (cause: %v)", rawValue, err)
	}
	parsed = math.Round(conversion(parsed)*100.0) / 100.0
	return parsed, nil
}

func (plugin *HomeKit) processLightData(dataType string, dataRoom string, dataName string, value string) {
	light, err := strconv.Atoi(value)
	if err != nil {
		plugin.Log.Warnf("Failed to process light data: %s (cause: %v)", value, err)
		return
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_room"] = dataRoom
	tags["homekit_accessory"] = dataName
	fields := make(map[string]interface{})
	fields["light"] = light
	plugin.acc.AddCounter("homekit_light", fields, tags)
}

func (plugin *HomeKit) processLightLevelData(dataType string, dataRoom string, dataName string, value string) {
	level, err := plugin.parseLightLevelValue(value)
	if err != nil {
		plugin.Log.Warnf("Failed to process light level data: %s (cause: %v)", value, err)
		return
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_room"] = dataRoom
	tags["homekit_accessory"] = dataName
	fields := make(map[string]interface{})
	fields["level"] = level
	plugin.acc.AddCounter("homekit_lightlevel", fields, tags)
}

func (plugin *HomeKit) parseLightLevelValue(value string) (float64, error) {
	parsed := 0.0
	lxSuffix := " lx"
	suffix := lxSuffix
	conversion := func(c float64) float64 { return c }
	if !strings.HasSuffix(value, lxSuffix) {
		return parsed, fmt.Errorf("unrecognized light level value: %s", value)
	}
	rawValue := strings.TrimSuffix(value, suffix)
	rawValue = strings.ReplaceAll(rawValue, ",", ".")
	parsed, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return parsed, fmt.Errorf("failed to parse light level value: %s (cause: %v)", rawValue, err)
	}
	parsed = math.Round(conversion(parsed)*100.0) / 100.0
	return parsed, nil
}

func (plugin *HomeKit) processStateData(dataType string, dataRoom string, dataName string, value string) {
	state := false
	for _, stateValue := range plugin.ActiveStateValues {
		if value == stateValue {
			state = true
			break
		}
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_room"] = dataRoom
	tags["homekit_accessory"] = dataName
	fields := make(map[string]interface{})
	fields["state"] = state
	plugin.acc.AddCounter("homekit_state", fields, tags)
}

func init() {
	inputs.Add("homekit", func() telegraf.Input {
		return NewHomeKit()
	})
}
