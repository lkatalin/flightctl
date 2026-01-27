package tpm

import (
	"errors"
	"os"
	"testing"

	"github.com/flightctl/flightctl/internal/agent/device/fileio"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestReadIMAMeasurements(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*fileio.MockReadWriter)
		expectedData  string
		expectedError bool
	}{
		{
			name: "successful read",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(IMAMeasurementPath).Return(true, nil)
				m.EXPECT().ReadFile(IMAMeasurementPath).Return([]byte("10 hash1 ima-ng sha256:abc /bin/ls\n"), nil)
			},
			expectedData:  "10 hash1 ima-ng sha256:abc /bin/ls\n",
			expectedError: false,
		},
		{
			name: "IMA not available",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(IMAMeasurementPath).Return(false, nil)
			},
			expectedData:  "",
			expectedError: false,
		},
		{
			name: "path check error",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(IMAMeasurementPath).Return(false, errors.New("permission denied"))
			},
			expectedData:  "",
			expectedError: true,
		},
		{
			name: "read file permission error returns empty",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(IMAMeasurementPath).Return(true, nil)
				m.EXPECT().ReadFile(IMAMeasurementPath).Return(nil, &os.PathError{Op: "open", Path: IMAMeasurementPath, Err: os.ErrPermission})
			},
			expectedData:  "",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRW := fileio.NewMockReadWriter(ctrl)
			tt.setupMock(mockRW)

			data, err := ReadIMAMeasurements(mockRW)

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedData, data)
			}
		})
	}
}

func TestReadMeasuredBootLog(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*fileio.MockReadWriter)
		expectedData  []byte
		expectedError bool
	}{
		{
			name: "successful read from first path",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(MeasuredBootLogPath).Return(true, nil)
				m.EXPECT().ReadFile(MeasuredBootLogPath).Return([]byte{0x00, 0x01, 0x02}, nil)
			},
			expectedData:  []byte{0x00, 0x01, 0x02},
			expectedError: false,
		},
		{
			name: "successful read from second path",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(MeasuredBootLogPath).Return(false, nil)
				m.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(true, nil)
				m.EXPECT().ReadFile("/sys/kernel/security/tpm0/binary_bios_measurements").Return([]byte{0x00, 0x01, 0x02}, nil)
			},
			expectedData:  []byte{0x00, 0x01, 0x02},
			expectedError: false,
		},
		{
			name: "no measured boot log available",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists(MeasuredBootLogPath).Return(false, nil)
				m.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(false, nil)
				m.EXPECT().PathExists("/sys/class/tpm/tpm0/device/binary_bios_measurements").Return(false, nil)
			},
			expectedData:  nil,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRW := fileio.NewMockReadWriter(ctrl)
			tt.setupMock(mockRW)

			data, err := ReadMeasuredBootLog(mockRW)

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedData, data)
			}
		})
	}
}

func TestValidateAttestationData(t *testing.T) {
	tests := []struct {
		name          string
		quote         []byte
		signature     []byte
		pcrs          []byte
		nonce         []byte
		expectedError string
	}{
		{
			name:          "valid attestation data",
			quote:         []byte("quote"),
			signature:     []byte("signature"),
			pcrs:          []byte("pcrs"),
			nonce:         make([]byte, 20),
			expectedError: "",
		},
		{
			name:          "empty quote",
			quote:         []byte{},
			signature:     []byte("signature"),
			pcrs:          []byte("pcrs"),
			nonce:         make([]byte, 20),
			expectedError: "quote is empty",
		},
		{
			name:          "empty signature",
			quote:         []byte("quote"),
			signature:     []byte{},
			pcrs:          []byte("pcrs"),
			nonce:         make([]byte, 20),
			expectedError: "signature is empty",
		},
		{
			name:          "empty PCRs",
			quote:         []byte("quote"),
			signature:     []byte("signature"),
			pcrs:          []byte{},
			nonce:         make([]byte, 20),
			expectedError: "PCRs are empty",
		},
		{
			name:          "nonce too short",
			quote:         []byte("quote"),
			signature:     []byte("signature"),
			pcrs:          []byte("pcrs"),
			nonce:         make([]byte, 7),
			expectedError: "nonce must be at least 8 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAttestationData(tt.quote, tt.signature, tt.pcrs, tt.nonce)

			if tt.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetMeasuredBootLogPath(t *testing.T) {
	tests := []struct {
		name         string
		setupMock    func(*fileio.MockReadWriter)
		expectedPath string
	}{
		{
			name: "finds first path",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(true, nil)
			},
			expectedPath: "/sys/kernel/security/tpm0/binary_bios_measurements",
		},
		{
			name: "finds second path",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(false, nil)
				m.EXPECT().PathExists("/sys/class/tpm/tpm0/device/binary_bios_measurements").Return(true, nil)
			},
			expectedPath: "/sys/class/tpm/tpm0/device/binary_bios_measurements",
		},
		{
			name: "no path found",
			setupMock: func(m *fileio.MockReadWriter) {
				m.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(false, nil)
				m.EXPECT().PathExists("/sys/class/tpm/tpm0/device/binary_bios_measurements").Return(false, nil)
			},
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRW := fileio.NewMockReadWriter(ctrl)
			tt.setupMock(mockRW)

			path, err := GetMeasuredBootLogPath(mockRW)

			require.NoError(t, err)
			require.Equal(t, tt.expectedPath, path)
		})
	}
}
