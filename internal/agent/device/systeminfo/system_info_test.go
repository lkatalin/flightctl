package systeminfo

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/flightctl/flightctl/internal/agent/device/fileio"
	"github.com/flightctl/flightctl/internal/util"
	"github.com/flightctl/flightctl/pkg/executer"
	"github.com/flightctl/flightctl/pkg/log"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestManager(t *testing.T) {
	require := require.New(t)

	// setup
	tmpDir := t.TempDir()
	dataDir := filepath.Join("etc", "flightctl")
	readWriter := fileio.NewReadWriter()
	readWriter.SetRootdir(tmpDir)
	err := readWriter.MkdirAll(dataDir, 0755)
	require.NoError(err)
	err = readWriter.MkdirAll("/proc/sys/kernel/random", 0755)
	require.NoError(err)
	log := log.NewPrefixLogger("test")

	// set mock boot_id
	mockBootID := "c4070599-f0f0-472d-8084-09b7274ebf18"
	err = readWriter.WriteFile(bootIDPath, []byte(mockBootID), 0644)
	require.NoError(err)

	ctrl := gomock.NewController(t)
	mockExecuter := executer.NewMockExecuter(ctrl)
	bootTime := "2024-12-13 11:01:08"
	collectTimeout := util.Duration(5 * time.Second)
	mockExecuter.EXPECT().Execute("uptime", "-s").Return(bootTime, "", 0).Times(2)

	// initialize client new device
	factKeys := []string{""}
	manager := NewManager(log, mockExecuter, readWriter, dataDir, factKeys, collectTimeout)
	err = manager.Initialize()
	require.NoError(err)
	require.NotNil(manager)
	require.NotEmpty(manager.BootTime())
	require.False(manager.IsRebooted())
	require.Equal(mockBootID, manager.BootID())

	// test rebooted
	// change bootID stored in system.json on disk
	mockBootID2 := "c4070599-f0f0-472d-8084-09b7274ebf19"
	mockStatus := &Boot{
		Time: bootTime,
		ID:   mockBootID2,
	}
	mockStatusBytes, err := json.Marshal(mockStatus)
	require.NoError(err)
	err = readWriter.WriteFile(filepath.Join(dataDir, SystemFileName), mockStatusBytes, 0644)
	require.NoError(err)

	// reinitialize client
	manager = NewManager(log, mockExecuter, readWriter, dataDir, factKeys, collectTimeout)
	err = manager.Initialize()
	require.NoError(err)
	require.NotEmpty(manager.BootTime())
	require.Equal(mockBootID, manager.BootID())
	require.True(manager.IsRebooted())
}

// go test -benchmem -run=^$ -bench ^BenchmarkCollectInfo$ -cpuprofile=cpu.pprof -memprofile=mem.pprof github.com/flightctl/flightctl/internal/agent/device/systeminfo
func BenchmarkCollectInfo(b *testing.B) {
	ctx := context.Background()
	log := log.NewPrefixLogger("test")
	exec := &executer.CommonExecuter{}
	reader := fileio.NewReadWriter()
	hardwareMapPath := "/var/lib/flightctl/hardware_map.json"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info, err := CollectInfo(ctx, log, exec, reader, hardwareMapPath)
		if err != nil {
			b.Fatalf("CollectInfo failed: %v", err)
		}
		// use value to prevent compiler optimization
		if info == nil {
			b.Fatal("Expected non-nil info")
		}
	}
}

func TestGenerateDetails(t *testing.T) {
	require := require.New(t)
	// Define test cases
	tests := []struct {
		name           string
		key            string
		keys           []string
		info           Info
		scripContent   []byte
		scriptName     string
		expectedValue  string
		expectedExists bool
	}{
		{
			name:         "override hostname exit 0",
			key:          "hostname",
			scriptName:   "hostname",
			scripContent: generateScriptBytes(0, "hostname_test", 0),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "hostname_test",
			expectedExists: true,
		},
		{
			name:         "override hostname exit 0 timeout",
			key:          "hostname",
			scriptName:   "hostname",
			scripContent: generateScriptBytes(200, "hostname_test", 0),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "test-host",
			expectedExists: true,
		},
		{
			name:         "override hostname with script extension exit 0",
			key:          "hostname",
			scriptName:   "hostname.sh",
			scripContent: generateScriptBytes(0, "hostname_test", 0),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "hostname_test",
			expectedExists: true,
		},
		{
			name:         "override hostname with script prefix exit 0",
			key:          "hostname",
			scriptName:   "01-hostname",
			scripContent: generateScriptBytes(0, "hostname_test", 0),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "hostname_test",
			expectedExists: true,
		},
		{
			name:         "override hostname with script prefix and suffix exit 0",
			key:          "hostname",
			scriptName:   "01-hostname.sh",
			scripContent: generateScriptBytes(0, "hostname_test", 0),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "hostname_test",
			expectedExists: true,
		},
		{
			name:         "override hostname with invalid script name",
			key:          "hostname",
			scriptName:   "01-hostname_custom.sh",
			scripContent: generateScriptBytes(0, "hostname_test", 0),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "test-host",
			expectedExists: true,
		},
		{
			name:         "override hostname exit 1",
			key:          "hostname",
			scriptName:   "hostname",
			scripContent: generateScriptBytes(0, "hostname_test", 1),
			info: Info{
				Hostname: "test-host",
			},
			keys:           []string{"hostname"},
			expectedValue:  "test-host",
			expectedExists: true,
		},
		{
			name:           "empty info override hostname",
			info:           Info{},
			key:            "hostname",
			scriptName:     "hostname",
			scripContent:   generateScriptBytes(0, "hostname_test", 0),
			keys:           []string{"hostname"},
			expectedValue:  "hostname_test",
			expectedExists: true,
		},
		{
			name: "undefined custom key",
			info: Info{
				Hostname: "test-host",
			},
			key:            "custom_key",
			scriptName:     "custom_key",
			scripContent:   generateScriptBytes(0, "custom_value", 0),
			keys:           []string{"hostname"},
			expectedValue:  "",
			expectedExists: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			tmpDir := t.TempDir()
			dataDir := filepath.Join(tmpDir, "var", "lib", "flightctl")

			rw := fileio.NewReadWriter()
			rw.SetRootdir(tmpDir)
			overrideDir := filepath.Join(dataDir, PluginDir)
			err := rw.MkdirAll(overrideDir, fileio.DefaultDirectoryPermissions)
			require.NoError(err)
			scriptFile := filepath.Join(overrideDir, tt.scriptName)

			err = rw.WriteFile(scriptFile, tt.scripContent, fileio.DefaultExecutablePermissions)
			require.NoError(err)
			log := log.NewPrefixLogger("test")
			exec := &executer.CommonExecuter{}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			details := GenerateDetails(ctx, log, rw, exec, &tt.info, tt.keys, dataDir)
			require.NotEmpty(details)
			value, exists := details[tt.key]
			require.Equal(tt.expectedExists, exists)
			require.Equal(tt.expectedValue, value)
		})
	}
}

func generateScriptBytes(sleepms int, output string, exitCode int) []byte {
	var sleepCmd string
	if sleepms > 0 {
		// generate sleep
		sleepCmd = fmt.Sprintf("sleep 0.%03d\n", sleepms)
	}

	content := fmt.Sprintf("#!/bin/bash\n%secho '%s'\nexit %d", sleepCmd, output, exitCode)
	return []byte(content)
}
