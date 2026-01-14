// Copyright 2026 Google Inc. All Rights Reserved.
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

package activity

import (
	"strings"
	"testing"
)

// bugreportHeader returns the standard bugreport header with timezone info.
func bugreportHeader() string {
	return strings.Join([]string{
		"========================================================",
		"== dumpstate: 2015-09-27 20:44:59",
		"========================================================",
		"",
		"------ TIMEZONE ------",
		"America/Los_Angeles",
		"",
	}, "\n")
}

// TestAOSPEnhancedEvents tests the newly added AOSP documentation-based event parsing.
func TestAOSPEnhancedEvents(t *testing.T) {
	tests := []struct {
		desc     string
		logLines []string
		wantDesc string
		wantVal  string
	}{
		{
			desc: "am_focused_activity tracking",
			logLines: []string{
				"09-27 20:44:00.000  4600 14112 I am_focused_activity: [0,com.google.android.GoogleCamera/com.android.camera.CameraActivity]",
			},
			wantDesc: "Focused Activity",
			wantVal:  "com.google.android.GoogleCamera/com.android.camera.CameraActivity",
		},
		{
			desc: "BLE Scanner registration (Android 7.0+)",
			logLines: []string{
				"09-27 20:45:00.000  24840 24851 D BluetoothLeScanner: onClientRegistered() - status=0 clientIf=5",
			},
			wantDesc: "BLE Scanner Registered",
			wantVal:  "Unknown PID 24840",
		},
		{
			desc: "ActivityManager ANR detection",
			logLines: []string{
				"09-27 20:46:00.000  1963  1976 E ActivityManager: ANR in com.example.app",
			},
			wantDesc: "ANR Detected",
			wantVal:  "ANR in com.example.app",
		},
		{
			desc: "Process killed due to low memory",
			logLines: []string{
				"09-27 20:47:00.000  1963  1976 I ActivityManager: Killing 30363:com.google.android.apps.plus/u0a206 (adj 0): bg anr",
			},
			wantDesc: "Process Killed (Low Memory)",
			wantVal:  "Killing 30363:com.google.android.apps.plus/u0a206 (adj 0): bg anr",
		},
		{
			desc: "Slow broadcast detection",
			logLines: []string{
				"09-27 20:48:00.000  1963  1976 W ActivityManager: Broadcast of Intent { act=android.intent.action.BATTERY_CHANGED } took 5000ms",
			},
			wantDesc: "Slow Broadcast",
			wantVal:  "Broadcast of Intent",
		},
		{
			desc: "System watchdog trigger",
			logLines: []string{
				"09-27 20:49:00.000  1963  1976 W ActivityManager: WATCHDOG KILLING SYSTEM PROCESS",
			},
			wantDesc: "System Watchdog",
			wantVal:  "WATCHDOG KILLING SYSTEM PROCESS",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			// Build complete bugreport input
			parts := []string{
				bugreportHeader(),
				"------ SYSTEM LOG (logcat -v threadtime -d *:v) ------",
				"--------- beginning of system",
			}
			parts = append(parts, test.logLines...)
			input := strings.Join(parts, "\n")

			result := Parse(nil, input)

			// Check that we got some CSV output for system log
			systemLog, ok := result.Logs[SystemLogSection]
			if !ok || systemLog == nil {
				t.Fatalf("Parse(%s) got no system log section", test.desc)
			}

			csv := systemLog.CSV
			if csv == "" {
				t.Fatalf("Parse(%s) got empty CSV for system log", test.desc)
			}

			// Verify the CSV contains our expected event description
			if !strings.Contains(csv, test.wantDesc) {
				t.Errorf("Parse(%s) CSV missing event description\nGot CSV:\n%s\nWant to contain: %s",
					test.desc, csv, test.wantDesc)
			}

			// Verify the CSV contains our expected value (partial match)
			if !strings.Contains(csv, test.wantVal) {
				t.Errorf("Parse(%s) CSV missing expected value\nGot CSV:\n%s\nWant to contain: %s",
					test.desc, csv, test.wantVal)
			}
		})
	}
}

// TestBluetoothScanStopTracking tests BLE scan stop event tracking.
func TestBluetoothScanStopTracking(t *testing.T) {
	input := strings.Join([]string{
		bugreportHeader(),
		"------ SYSTEM LOG (logcat -v threadtime -d *:v) ------",
		"--------- beginning of system",
		"09-27 20:44:00.000  12345  12346 D BluetoothAdapter: startLeScan()",
		"09-27 20:44:05.000  12345  12346 D BluetoothAdapter: stopLeScan()",
	}, "\n")

	result := Parse(nil, input)
	systemLog, ok := result.Logs[SystemLogSection]
	if !ok || systemLog == nil {
		t.Fatal("Parse() got no system log section")
	}

	csv := systemLog.CSV

	// Should have both start and stop events
	if !strings.Contains(csv, "Bluetooth Scan,") {
		t.Error("Parse() CSV missing Bluetooth Scan event")
	}
	if !strings.Contains(csv, "Bluetooth Scan Stopped,") {
		t.Error("Parse() CSV missing Bluetooth Scan Stopped event")
	}
}
