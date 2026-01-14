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

// Package battery_history_format_v2 provides parsing support for Android Battery History Format 2.
// This code is part of the parseutils package.
package parseutils

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/battery-historian/csv"
)

// BatteryHistoryV2 provides parsing support for Android Battery History Format 2.
// Format 2 was introduced in Android 12 and provides human-readable battery stats history.
// Example Format 2 line:
//   01-11 12:11:14.405 075 c4002820 status=discharging health=good plug=none temp=254 volt=4170 charge=3887

// BatteryHistoryV2Entry represents a parsed line from Battery History Format 2
type BatteryHistoryV2Entry struct {
	Timestamp           time.Time
	TimestampMs         int64
	BatteryPercent      int32
	Voltage             int32
	Temperature         int32
	ChargeMicroAh       int64
	Status              string
	Health              string
	PlugType            string
	DataConn            string
	PhoneSignalStrength string
	WiFiSignalStrength  int32
	WiFiSupplicantState string
	DeviceIdleMode      string
	States              map[string]bool  // e.g., "+running", "-wifi"
	WakeReasons         map[string]bool  // e.g., "wlan_wake", "rtc_alarm"
	RailCharges         map[string]int64 // e.g., "modemRailChargemAh"
}

var (
	// Format 2 battery history line pattern
	// Example: 01-11 12:11:14.405 075 c4002820 status=discharging health=good ...
	historyLinePatternV2 = regexp.MustCompile(`^(\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2}\.\d{3})\s+(\d+)\s+([0-9a-f]+)\s+(.*)$`)

	// Pattern for key=value pairs
	keyValuePattern = regexp.MustCompile(`(\w+)=([^,\s]+)`)

	// Pattern for state transitions (+state or -state)
	stateTransitionPattern = regexp.MustCompile(`([+-])(\w+)`)

	// Pattern for wake_reason=0:"reason_string"
	wakeReasonPattern = regexp.MustCompile(`wake_reason=\d+:"([^"]+)"`)
)

// ParseHistoryV2Line parses a single line from Battery History Format 2
func ParseHistoryV2Line(line string) (*BatteryHistoryV2Entry, error) {
	matches := historyLinePatternV2.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) == 0 {
		return nil, errors.New("invalid battery history v2 format")
	}

	entry := &BatteryHistoryV2Entry{
		States:      make(map[string]bool),
		WakeReasons: make(map[string]bool),
		RailCharges: make(map[string]int64),
	}

	// Parse timestamp (e.g., "01-11 12:11:14.405")
	monthDay := matches[1]
	timeStr := matches[2]
	// Note: We don't have year information, so we use current year (would need context in real impl)
	timestampStr := fmt.Sprintf("2026-%s %s", monthDay, timeStr)
	ts, err := time.Parse("2006-01-02 15:04:05.000", timestampStr)
	if err != nil {
		// Return error but continue parsing
		entry.Timestamp = time.Now()
	} else {
		entry.Timestamp = ts
	}

	// Parse remainder of line for key=value pairs and state transitions
	remainder := matches[5]
	parseStateTransitionsV2(entry, remainder)
	parseKeyValuePairsV2(entry, remainder)
	parseWakeReasonsV2(entry, remainder)

	return entry, nil
}

// parseKeyValuePairsV2 extracts all key=value pairs from the history line
func parseKeyValuePairsV2(entry *BatteryHistoryV2Entry, line string) {
	matches := keyValuePattern.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		key := match[1]
		value := match[2]

		switch key {
		case "charge":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				entry.ChargeMicroAh = v
			}
		case "volt":
			if v, err := strconv.ParseInt(value, 10, 32); err == nil {
				entry.Voltage = int32(v)
			}
		case "temp":
			if v, err := strconv.ParseInt(value, 10, 32); err == nil {
				entry.Temperature = int32(v)
			}
		case "status":
			entry.Status = value
		case "health":
			entry.Health = value
		case "plug":
			entry.PlugType = value
		case "data_conn":
			entry.DataConn = value
		case "phone_signal_strength":
			entry.PhoneSignalStrength = value
		case "wifi_signal_strength":
			if v, err := strconv.ParseInt(value, 10, 32); err == nil {
				entry.WiFiSignalStrength = int32(v)
			}
		case "wifi_suppl":
			entry.WiFiSupplicantState = value
		case "device_idle":
			entry.DeviceIdleMode = value
		case "modemRailChargemAh", "wifiRailChargemAh":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				entry.RailCharges[key] = v
			}
		}
	}
}

// parseStateTransitionsV2 extracts state transitions (+state or -state)
func parseStateTransitionsV2(entry *BatteryHistoryV2Entry, line string) {
	matches := stateTransitionPattern.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		transition := match[1] // +/- sign
		state := match[2]      // state name

		// Only include state changes, not wake_lock or other key=value constructs
		if !strings.Contains(match[0], "=") {
			isActive := transition == "+"
			// Filter out partial matches that are part of larger tokens
			// by checking if they're bounded by whitespace or special chars
			idx := strings.Index(line, match[0])
			if idx >= 0 {
				// Check previous character if not at the start
				prevOk := true
				if idx > 0 {
					prevChar := line[idx-1]
					prevOk = prevChar == ' ' || prevChar == '+' || prevChar == '-'
				}

				// Check next character if not at the end
				nextOk := true
				if idx+len(match[0]) < len(line) {
					nextChar := line[idx+len(match[0])]
					nextOk = nextChar == ' ' || nextChar == '+' || nextChar == '-' || nextChar == ','
				}

				if prevOk && nextOk {
					entry.States[state] = isActive
				}
			}
		}
	}
}

// parseWakeReasonsV2 extracts wake reasons from the history line
func parseWakeReasonsV2(entry *BatteryHistoryV2Entry, line string) {
	matches := wakeReasonPattern.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		reason := match[1]
		entry.WakeReasons[reason] = true
	}
}

// ConvertToCSVEntry converts a V2 history entry to CSV format for backward compatibility
func (entry *BatteryHistoryV2Entry) ConvertToCSVEntry() csv.Entry {
	// Build value string from important fields
	values := []string{}
	if entry.Status != "" {
		values = append(values, fmt.Sprintf("status=%s", entry.Status))
	}
	if entry.Health != "" {
		values = append(values, fmt.Sprintf("health=%s", entry.Health))
	}
	if entry.Voltage > 0 {
		values = append(values, fmt.Sprintf("volt=%d", entry.Voltage))
	}
	if entry.Temperature > 0 {
		values = append(values, fmt.Sprintf("temp=%d", entry.Temperature))
	}

	return csv.Entry{
		Desc:       "Battery state change",
		Type:       "Battery State",
		Start:      entry.TimestampMs,
		Value:      strings.Join(values, ","),
		Identifier: "system",
	}
}

// DetectHistoryFormatVersion detects whether the battery history is Format 1 or Format 2
func DetectHistoryFormatVersion(historyText string) int {
	// Format 2 uses readable format with timestamps like "01-11 12:11:14.405"
	// Format 1 uses numeric format "9,h,0,Bl=..."
	lines := strings.Split(historyText, "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "9,h,") {
			return 1 // Classic format
		}
		if historyLinePatternV2.MatchString(line) {
			return 2 // Modern format
		}
	}
	return 1 // Default to format 1
}
