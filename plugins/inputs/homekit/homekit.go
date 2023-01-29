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
	CelsiusSuffixes       []string `toml:"celsius_suffixex"`
	FahrenheitSuffixes    []string `toml:"fahrenheit_suffixes"`
	LuxSuffixes           []string `toml:"lux_suffixes"`
	HueSuffixes           []string `toml:"hue_suffixes"`
	ActiveValues          []string `toml:"active_values"`
	InactiveValues        []string `toml:"inactive_values"`
	Debug                 bool     `toml:"debug"`
	HAPDebug              bool     `toml:"hap_debug"`
	DNSSDDebug            bool     `toml:"dnssd_debug"`

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
		CelsiusSuffixes:       []string{" °C"},
		FahrenheitSuffixes:    []string{" °F"},
		LuxSuffixes:           []string{" lx"},
		HueSuffixes:           []string{"°"},
		ActiveValues:          []string{"Yes", "Ja"},
		InactiveValues:        []string{"No", "Nein"}}
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
  ## Active values
  # active_values = ["Yes", "Ja"]
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
	if plugin.HAPDebug {
		haplog.Debug.Enable()
	}
	if plugin.DNSSDDebug {
		dnssdlog.Debug.Enable()
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
	if plugin.Debug {
		res.Write(bodyBytes)
	} else {
		res.Write([]byte("Ok"))
	}
}

func (plugin *HomeKit) processData(data map[string]string) {
	for key, value := range data {
		keyParts := strings.SplitN(key, "_", 3)
		name := ""
		room := "undefined"
		characteristic := "generic"
		switch len(keyParts) {
		case 1:
			name = keyParts[0]
		case 2:
			name = keyParts[0]
			room = keyParts[1]
		case 3:
			name = keyParts[0]
			room = keyParts[1]
			characteristic = keyParts[2]
		default:
			plugin.Log.Warnf("Ignoring invalid data key: %s = %s", key, value)
			continue
		}
		err := plugin.processDataValue(name, room, characteristic, value)
		if err != nil {
			plugin.Log.Warnf("Ignoreing invalid data value: %s = %s (cause: %v)", key, value, err)
		}
	}
}

func (plugin *HomeKit) processDataValue(name string, room string, characteristic string, value string) error {
	for _, celsiusSuffix := range plugin.CelsiusSuffixes {
		if strings.HasSuffix(value, celsiusSuffix) {
			return plugin.processCelsiusValue(name, room, characteristic, value, celsiusSuffix)
		}
	}
	for _, fahrenheitSuffix := range plugin.FahrenheitSuffixes {
		if strings.HasSuffix(value, fahrenheitSuffix) {
			return plugin.processFahrenheitValue(name, room, characteristic, value, fahrenheitSuffix)
		}
	}
	for _, luxSuffix := range plugin.LuxSuffixes {
		if strings.HasSuffix(value, luxSuffix) {
			return plugin.processLuxValue(name, room, characteristic, value, luxSuffix)
		}
	}
	for _, hueSuffix := range plugin.HueSuffixes {
		if strings.HasSuffix(value, hueSuffix) {
			return plugin.processHueValue(name, room, characteristic, value, hueSuffix)
		}
	}
	for _, activeValue := range plugin.ActiveValues {
		if value == activeValue {
			return plugin.processStateValue(name, room, characteristic, true)
		}
	}
	for _, inactiveValue := range plugin.InactiveValues {
		if value == inactiveValue {
			return plugin.processStateValue(name, room, characteristic, false)
		}
	}
	return fmt.Errorf("unrecognized value type")
}

func (plugin *HomeKit) processCelsiusValue(name string, room string, characteristic string, value string, suffix string) error {
	celsiusValue := strings.TrimSuffix(value, suffix)
	celsius, err := plugin.parseFloat(celsiusValue)
	if err != nil {
		return err
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_name"] = name
	tags["homekit_room"] = room
	tags["homekit_characteristic"] = characteristic
	fields := make(map[string]interface{})
	fields["celsius"] = celsius
	fields["fahrenheit"] = (celsius * 1.8) + 32.0
	plugin.acc.AddCounter("homekit_temperature", fields, tags)
	return nil
}

func (plugin *HomeKit) processFahrenheitValue(name string, room string, characteristic string, value string, suffix string) error {
	fahrenheitValue := strings.TrimSuffix(value, suffix)
	fahrenheit, err := plugin.parseFloat(fahrenheitValue)
	if err != nil {
		return err
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_name"] = name
	tags["homekit_room"] = room
	tags["homekit_characteristic"] = characteristic
	fields := make(map[string]interface{})
	fields["celsius"] = (fahrenheit - 32.0) / 1.8
	fields["fahrenheit"] = fahrenheit
	plugin.acc.AddCounter("homekit_temperature", fields, tags)
	return nil
}

func (plugin *HomeKit) processLuxValue(name string, room string, characteristic string, value string, suffix string) error {
	luxValue := strings.TrimSuffix(value, suffix)
	lux, err := plugin.parseFloat(luxValue)
	if err != nil {
		return err
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_name"] = name
	tags["homekit_room"] = room
	tags["homekit_characteristic"] = characteristic
	fields := make(map[string]interface{})
	fields["lux"] = lux
	plugin.acc.AddCounter("homekit_lightlevel", fields, tags)
	return nil
}

func (plugin *HomeKit) processHueValue(name string, room string, characteristic string, value string, suffix string) error {
	hueValue := strings.TrimSuffix(value, suffix)
	hue, err := strconv.Atoi(hueValue)
	if err != nil {
		return err
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_name"] = name
	tags["homekit_room"] = room
	tags["homekit_characteristic"] = characteristic
	fields := make(map[string]interface{})
	fields["hue"] = hue
	plugin.acc.AddCounter("homekit_lighthue", fields, tags)
	return nil
}

func (plugin *HomeKit) processStateValue(name string, room string, characteristic string, active bool) error {
	var percent int
	if active {
		percent = 100
	} else {
		percent = 0
	}
	tags := make(map[string]string)
	tags["homekit_monitor"] = plugin.MonitorAccessoryName
	tags["homekit_name"] = name
	tags["homekit_room"] = room
	tags["homekit_characteristic"] = characteristic
	fields := make(map[string]interface{})
	fields["active"] = active
	fields["percent"] = percent
	plugin.acc.AddCounter("homekit_state", fields, tags)
	return nil
}

func (plugin *HomeKit) parseFloat(value string) (float64, error) {
	comma := strings.LastIndex(value, ",")
	cValue := value
	if comma >= 0 {
		cValue = strings.ReplaceAll(cValue, ",", ".")
	}
	return strconv.ParseFloat(cValue, 64)
}

func init() {
	inputs.Add("homekit", func() telegraf.Input {
		return NewHomeKit()
	})
}
