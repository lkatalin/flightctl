package service

import (
	"time"

	"github.com/flightctl/flightctl/internal/domain"
	"github.com/sirupsen/logrus"
)

// AttestationMetadata contains additional metadata about the attestation process
type AttestationMetadata struct {
	// ReceivedAt is when the server received the attestation data
	ReceivedAt time.Time
	// EnrollmentRequestName is the name of the enrollment request this attestation belongs to
	EnrollmentRequestName string
	// DeviceName is the device name from the enrollment request (same as enrollment request name)
	DeviceName string
}

// AttestationPackage wraps attestation data with metadata for processing
type AttestationPackage struct {
	// Data is the raw attestation data from the enrollment request
	Data *domain.AttestationData
	// Metadata contains additional information about when and how the attestation was received
	Metadata AttestationMetadata
}

// extractAttestationData extracts and wraps attestation data from an enrollment request
// Returns nil if no attestation data is present
func extractAttestationData(er *domain.EnrollmentRequest, log logrus.FieldLogger) *AttestationPackage {
	if er.Spec.AttestationData == nil {
		return nil
	}

	deviceName := *er.Metadata.Name

	pkg := &AttestationPackage{
		Data: er.Spec.AttestationData,
		Metadata: AttestationMetadata{
			ReceivedAt:            time.Now(),
			EnrollmentRequestName: deviceName,
			DeviceName:            deviceName,
		},
	}

	// Log that we received attestation data
	log.Infof("Received attestation data for enrollment request %s (device: %s): quote=%d bytes, AK=%d bytes, EK=%d bytes, hash=%s, IMA=%d bytes, MB=%d bytes",
		pkg.Metadata.EnrollmentRequestName,
		pkg.Metadata.DeviceName,
		len(getStringValue(pkg.Data.Quote)),
		len(getStringValue(pkg.Data.TpmAk)),
		len(getStringValue(pkg.Data.TpmEk)),
		getStringValue(pkg.Data.HashAlg),
		len(getStringValue(pkg.Data.ImaMeasurementList)),
		len(getStringValue(pkg.Data.MbLog)),
	)

	return pkg
}

// getStringValue safely returns the value of a string pointer or empty string if nil
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
