// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parseutils

import (
	"strings"
	"testing"
)

// TestParseHistoryV2Line tests parsing of individual Battery History Format 2 lines
func TestParseHistoryV2Line(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		checks  func(*BatteryHistoryV2Entry) bool
	}{
		{
			name:    "Basic battery state with key-value pairs",
			line:    `01-11 12:11:14.405 075 c4002820 status=discharging health=good plug=none temp=254 volt=4170 charge=3887`,
			wantErr: false,
			checks: func(e *BatteryHistoryV2Entry) bool {
				return e.Status == "discharging" && e.Health == "good" && e.Voltage == 4170 && e.Temperature == 254
			},
		},
		{
			name:    "State transitions and wake lock",
			line:    `01-11 12:11:14.446 075 84002820 -wake_lock=u0a231:"*alarm*" -cellular_high_tx_power`,
			wantErr: false,
			checks: func(e *BatteryHistoryV2Entry) bool {
				// Should parse without error even with complex wake_lock format
				return e != nil
			},
		},
		{
			name:    "Running state with wake reason",
			line:    `01-11 12:11:15.396 075 84002820 +running wake_reason=0:"100 wlan_wake"`,
			wantErr: false,
			checks: func(e *BatteryHistoryV2Entry) bool {
				// Should detect wake reason
				return len(e.WakeReasons) > 0
			},
		},
		{
			name:    "WiFi and network data connections",
			line:    `01-11 12:11:14.405 075 c4002820 +wifi +wifi_radio data_conn=nr phone_signal_strength=great wifi_signal_strength=4`,
			wantErr: false,
			checks: func(e *BatteryHistoryV2Entry) bool {
				return e.DataConn == "nr" && e.PhoneSignalStrength == "great" && e.WiFiSignalStrength == 4
			},
		},
		{
			name:    "Rail charge measurements",
			line:    `01-11 12:11:14.405 075 c4002820 modemRailChargemAh=0 wifiRailChargemAh=0 status=discharging`,
			wantErr: false,
			checks: func(e *BatteryHistoryV2Entry) bool {
				modemRail, hasModem := e.RailCharges["modemRailChargemAh"]
				wifiRail, hasWiFi := e.RailCharges["wifiRailChargemAh"]
				return hasModem && hasWiFi && modemRail == 0 && wifiRail == 0
			},
		},
		{
			name:    "Device idle mode",
			line:    `01-11 12:11:14.405 075 c4002820 device_idle=full wifi_signal_strength=4 wifi_suppl=completed`,
			wantErr: false,
			checks: func(e *BatteryHistoryV2Entry) bool {
				return e.DeviceIdleMode == "full" && e.WiFiSupplicantState == "completed"
			},
		},
		{
			name:    "Invalid format should error",
			line:    `invalid line format`,
			wantErr: true,
			checks: func(e *BatteryHistoryV2Entry) bool {
				return e == nil
			},
		},
		{
			name:    "Empty line should error",
			line:    ``,
			wantErr: true,
			checks: func(e *BatteryHistoryV2Entry) bool {
				return e == nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHistoryV2Line(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHistoryV2Line() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !tt.checks(got) {
				t.Errorf("ParseHistoryV2Line() checks failed for line: %s", tt.line)
			}
		})
	}
}

// TestDetectHistoryFormatVersion tests automatic format detection
func TestDetectHistoryFormatVersion(t *testing.T) {
	tests := []struct {
		name    string
		history string
		want    int
	}{
		{
			name: "Format 1 (classic numeric format)",
			history: `9,h,0,Bl=52
9,h,100,+running
9,h,200,-running`,
			want: 1,
		},
		{
			name: "Format 2 (modern readable format)",
			history: `01-11 12:11:14.405 075 c4002820 status=discharging health=good
01-11 12:11:15.396 075 84002820 +running wake_reason=0:"wlan_wake"`,
			want: 2,
		},
		{
			name:    "Empty history defaults to Format 1",
			history: "",
			want:    1,
		},
		{
			name: "Mixed formats (should detect Format 1 line first)",
			history: `9,h,0,Bl=52
01-11 12:11:14.405 075 c4002820 status=discharging`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectHistoryFormatVersion(tt.history)
			if got != tt.want {
				t.Errorf("DetectHistoryFormatVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestParseStateTransitionsV2 tests extraction of +state and -state transitions
func TestParseStateTransitionsV2(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantKey string
		wantVal bool
	}{
		{
			name:    "Positive state transition",
			line:    "+running +wifi +ble_scan",
			wantKey: "running",
			wantVal: true,
		},
		{
			name:    "Negative state transition",
			line:    "-running -wifi",
			wantKey: "running",
			wantVal: false,
		},
		{
			name:    "Mixed transitions",
			line:    "+running -wifi +device_idle=full",
			wantKey: "running",
			wantVal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &BatteryHistoryV2Entry{
				States:      make(map[string]bool),
				WakeReasons: make(map[string]bool),
				RailCharges: make(map[string]int64),
			}
			parseStateTransitionsV2(entry, tt.line)

			gotVal, exists := entry.States[tt.wantKey]
			if !exists {
				t.Errorf("parseStateTransitionsV2() state %q not found in line: %s", tt.wantKey, tt.line)
				return
			}
			if gotVal != tt.wantVal {
				t.Errorf("parseStateTransitionsV2() state %q = %v, want %v", tt.wantKey, gotVal, tt.wantVal)
			}
		})
	}
}

// TestParseKeyValuePairsV2 tests extraction of key=value fields
func TestParseKeyValuePairsV2(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantKey  string
		checkVal func(interface{}) bool
	}{
		{
			name:    "Battery voltage",
			line:    "volt=4170 temp=254 status=discharging",
			wantKey: "volt",
			checkVal: func(v interface{}) bool {
				return v.(int32) == 4170
			},
		},
		{
			name:    "Battery status",
			line:    "status=discharging health=good plug=none",
			wantKey: "status",
			checkVal: func(v interface{}) bool {
				return v.(string) == "discharging"
			},
		},
		{
			name:    "Rail charge measurements",
			line:    "modemRailChargemAh=150 wifiRailChargemAh=75",
			wantKey: "modemRailChargemAh",
			checkVal: func(v interface{}) bool {
				return v.(int64) == 150
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &BatteryHistoryV2Entry{
				States:      make(map[string]bool),
				WakeReasons: make(map[string]bool),
				RailCharges: make(map[string]int64),
			}
			parseKeyValuePairsV2(entry, tt.line)

			switch tt.wantKey {
			case "volt":
				if !tt.checkVal(entry.Voltage) {
					t.Errorf("parseKeyValuePairsV2() voltage check failed for line: %s", tt.line)
				}
			case "status":
				if !tt.checkVal(entry.Status) {
					t.Errorf("parseKeyValuePairsV2() status check failed for line: %s", tt.line)
				}
			case "modemRailChargemAh":
				if val, exists := entry.RailCharges["modemRailChargemAh"]; !exists || !tt.checkVal(val) {
					t.Errorf("parseKeyValuePairsV2() rail charge check failed for line: %s", tt.line)
				}
			}
		})
	}
}

// TestParseWakeReasonsV2 tests extraction of wake_reason fields
func TestParseWakeReasonsV2(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantReason string
	}{
		{
			name:       "WiFi host wake",
			line:       `wake_reason=0:"100 wlan_wake"`,
			wantReason: "100 wlan_wake",
		},
		{
			name:       "RTC wake",
			line:       `wake_reason=0:"100 rtc_alarm"`,
			wantReason: "100 rtc_alarm",
		},
		{
			name:       "Abort wake event",
			line:       `wake_reason=0:"Abort: Pending Wakeup Sources: wlan_rx_wake"`,
			wantReason: "Abort: Pending Wakeup Sources: wlan_rx_wake",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &BatteryHistoryV2Entry{
				States:      make(map[string]bool),
				WakeReasons: make(map[string]bool),
				RailCharges: make(map[string]int64),
			}
			parseWakeReasonsV2(entry, tt.line)

			if _, exists := entry.WakeReasons[tt.wantReason]; !exists {
				t.Errorf("parseWakeReasonsV2() reason %q not found in line: %s", tt.wantReason, tt.line)
			}
		})
	}
}

// TestConvertToCSVEntry tests conversion of V2 entries to CSV format for backward compatibility
func TestConvertToCSVEntry(t *testing.T) {
	entry := &BatteryHistoryV2Entry{
		Status:      "discharging",
		Health:      "good",
		Voltage:     4170,
		Temperature: 254,
		States:      make(map[string]bool),
		WakeReasons: make(map[string]bool),
		RailCharges: make(map[string]int64),
	}

	csvEntry := entry.ConvertToCSVEntry()

	if csvEntry.Type != "Battery State" {
		t.Errorf("ConvertToCSVEntry() Type = %q, want %q", csvEntry.Type, "Battery State")
	}

	if !strings.Contains(csvEntry.Value, "status=discharging") {
		t.Errorf("ConvertToCSVEntry() Value missing status: %s", csvEntry.Value)
	}

	if !strings.Contains(csvEntry.Value, "volt=4170") {
		t.Errorf("ConvertToCSVEntry() Value missing voltage: %s", csvEntry.Value)
	}
}

// TestModernBugreportIntegration tests with actual modern bugreport format samples
func TestModernBugreportIntegration(t *testing.T) {
	// Sample from Android 16 (API 36) bugreport with modern Battery History Format 2
	// Test data is synthetic and does not contain real device information
	modernHistory := `Battery History [Format: 2] (102% used, 4211KB used of 4096KB, 483 strings using 26KB):
01-11 12:11:14.405 075 c4002820 status=discharging health=good plug=none temp=254 volt=4170 charge=3887 modemRailChargemAh=0 wifiRailChargemAh=0 +running +wake_lock=1000:"*alarm*:TIME_TICK" +wifi_radio data_conn=nr phone_signal_strength=great +wifi device_idle=full wifi_signal_strength=4 wifi_suppl=completed +ble_scan +cellular_high_tx_power wake_reason=0:"100 rtc_alarm"
01-11 12:11:14.446 075 84002820 -wake_lock=u0a231:"*alarm*" -cellular_high_tx_power
01-11 12:11:14.858 075 04002820 -running
01-11 12:11:15.396 075 84002820 +running wake_reason=0:"100 wlan_wake"`

	version := DetectHistoryFormatVersion(modernHistory)
	if version != 2 {
		t.Errorf("DetectHistoryFormatVersion() for modern bugreport = %d, want 2", version)
	}

	// Extract first actual history line (skip header)
	lines := strings.Split(modernHistory, "\n")
	var firstHistoryLine string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "01-") {
			firstHistoryLine = line
			break
		}
	}

	if firstHistoryLine == "" {
		t.Fatal("No history line found in test data")
	}

	entry, err := ParseHistoryV2Line(firstHistoryLine)
	if err != nil {
		t.Fatalf("ParseHistoryV2Line() error = %v", err)
	}

	// Verify key fields were parsed
	if entry.Status != "discharging" {
		t.Errorf("Expected status=discharging, got %q", entry.Status)
	}
	if entry.Voltage != 4170 {
		t.Errorf("Expected voltage=4170, got %d", entry.Voltage)
	}
	if len(entry.RailCharges) == 0 {
		t.Error("Expected rail charge data to be parsed")
	}
	if len(entry.States) == 0 {
		t.Error("Expected state transitions to be parsed")
	}
}
