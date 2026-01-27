package tpm

import (
	"fmt"
	"os"

	"github.com/flightctl/flightctl/internal/agent/device/fileio"
)

const (
	// IMAMeasurementPath is the default path to IMA runtime measurements
	IMAMeasurementPath = "/sys/kernel/security/ima/ascii_runtime_measurements"
	// MeasuredBootLogPath is the default path to the measured boot log
	MeasuredBootLogPath = "/sys/kernel/security/tpm0/binary_bios_measurements"
)

// ReadIMAMeasurements reads the IMA runtime measurements from the kernel
func ReadIMAMeasurements(rw fileio.ReadWriter) (string, error) {
	// Check if IMA is enabled
	exists, err := rw.PathExists(IMAMeasurementPath)
	if err != nil {
		return "", fmt.Errorf("checking IMA path: %w", err)
	}
	if !exists {
		return "", nil // IMA not available, return empty string
	}

	data, err := rw.ReadFile(IMAMeasurementPath)
	if err != nil {
		// IMA might be enabled but empty
		if os.IsPermission(err) || os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading IMA measurements: %w", err)
	}

	return string(data), nil
}

// ReadMeasuredBootLog reads the measured boot binary log
func ReadMeasuredBootLog(rw fileio.ReadWriter) ([]byte, error) {
	// Try multiple possible paths for measured boot log
	possiblePaths := []string{
		MeasuredBootLogPath,
		"/sys/kernel/security/tpm0/binary_bios_measurements",
		"/sys/class/tpm/tpm0/device/binary_bios_measurements",
	}

	for _, path := range possiblePaths {
		exists, err := rw.PathExists(path)
		if err != nil {
			continue
		}
		if !exists {
			continue
		}

		data, err := rw.ReadFile(path)
		if err != nil {
			continue
		}

		return data, nil
	}

	// Measured boot log not available
	return nil, nil
}

// GetMeasuredBootLogPath finds the measured boot log path if it exists
func GetMeasuredBootLogPath(rw fileio.ReadWriter) (string, error) {
	possiblePaths := []string{
		"/sys/kernel/security/tpm0/binary_bios_measurements",
		"/sys/class/tpm/tpm0/device/binary_bios_measurements",
	}

	for _, path := range possiblePaths {
		exists, err := rw.PathExists(path)
		if err != nil {
			continue
		}
		if exists {
			return path, nil
		}
	}

	return "", nil
}

// ValidateAttestationData performs basic validation on attestation data
func ValidateAttestationData(quote, signature, pcrs []byte, nonce []byte) error {
	if len(quote) == 0 {
		return fmt.Errorf("quote is empty")
	}
	if len(signature) == 0 {
		return fmt.Errorf("signature is empty")
	}
	if len(pcrs) == 0 {
		return fmt.Errorf("PCRs are empty")
	}
	if len(nonce) < MinNonceLength {
		return fmt.Errorf("nonce must be at least %d bytes, got %d", MinNonceLength, len(nonce))
	}
	return nil
}
