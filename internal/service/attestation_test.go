package service

import (
	"testing"
	"time"

	"github.com/flightctl/flightctl/internal/domain"
	"github.com/flightctl/flightctl/pkg/log"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestExtractAttestationData(t *testing.T) {
	logger := log.NewPrefixLogger("test")
	erName := "test-device"

	tests := []struct {
		name              string
		enrollmentRequest *domain.EnrollmentRequest
		expectNil         bool
		validateResult    func(*testing.T, *AttestationPackage)
	}{
		{
			name: "successful extraction with all fields",
			enrollmentRequest: &domain.EnrollmentRequest{
				Metadata: domain.ObjectMeta{
					Name: &erName,
				},
				Spec: domain.EnrollmentRequestSpec{
					AttestationData: &domain.AttestationData{
						Quote:              lo.ToPtr("dGVzdC1xdW90ZQ=="),
						Nonce:              lo.ToPtr("dGVzdC1ub25jZQ=="),
						HashAlg:            lo.ToPtr("sha256"),
						TpmAk:              lo.ToPtr("dGVzdC1haw=="),
						TpmEk:              lo.ToPtr("dGVzdC1law=="),
						ImaMeasurementList: lo.ToPtr("10 hash ima-ng sha256:abc /bin/ls\n"),
						MbLog:              lo.ToPtr("AAEC"),
					},
				},
			},
			expectNil: false,
			validateResult: func(t *testing.T, pkg *AttestationPackage) {
				require.NotNil(t, pkg)
				require.NotNil(t, pkg.Data)
				require.Equal(t, erName, pkg.Metadata.EnrollmentRequestName)
				require.Equal(t, erName, pkg.Metadata.DeviceName)
				require.WithinDuration(t, time.Now(), pkg.Metadata.ReceivedAt, 5*time.Second)
				require.NotNil(t, pkg.Data.Quote)
				require.Equal(t, "dGVzdC1xdW90ZQ==", *pkg.Data.Quote)
				require.NotNil(t, pkg.Data.HashAlg)
				require.Equal(t, "sha256", *pkg.Data.HashAlg)
			},
		},
		{
			name: "no attestation data",
			enrollmentRequest: &domain.EnrollmentRequest{
				Metadata: domain.ObjectMeta{
					Name: &erName,
				},
				Spec: domain.EnrollmentRequestSpec{
					AttestationData: nil,
				},
			},
			expectNil: true,
		},
		{
			name: "attestation data with minimal fields",
			enrollmentRequest: &domain.EnrollmentRequest{
				Metadata: domain.ObjectMeta{
					Name: &erName,
				},
				Spec: domain.EnrollmentRequestSpec{
					AttestationData: &domain.AttestationData{
						Quote:   lo.ToPtr("dGVzdC1xdW90ZQ=="),
						Nonce:   lo.ToPtr("dGVzdC1ub25jZQ=="),
						HashAlg: lo.ToPtr("sha256"),
						TpmAk:   lo.ToPtr("dGVzdC1haw=="),
						TpmEk:   lo.ToPtr("dGVzdC1law=="),
						// IMA and MB logs are optional
						ImaMeasurementList: nil,
						MbLog:              nil,
					},
				},
			},
			expectNil: false,
			validateResult: func(t *testing.T, pkg *AttestationPackage) {
				require.NotNil(t, pkg)
				require.NotNil(t, pkg.Data)
				require.Nil(t, pkg.Data.ImaMeasurementList)
				require.Nil(t, pkg.Data.MbLog)
			},
		},
		{
			name: "attestation data with empty optional fields",
			enrollmentRequest: &domain.EnrollmentRequest{
				Metadata: domain.ObjectMeta{
					Name: &erName,
				},
				Spec: domain.EnrollmentRequestSpec{
					AttestationData: &domain.AttestationData{
						Quote:              lo.ToPtr("dGVzdC1xdW90ZQ=="),
						Nonce:              lo.ToPtr("dGVzdC1ub25jZQ=="),
						HashAlg:            lo.ToPtr("sha256"),
						TpmAk:              lo.ToPtr("dGVzdC1haw=="),
						TpmEk:              lo.ToPtr("dGVzdC1law=="),
						ImaMeasurementList: lo.ToPtr(""),
						MbLog:              lo.ToPtr(""),
					},
				},
			},
			expectNil: false,
			validateResult: func(t *testing.T, pkg *AttestationPackage) {
				require.NotNil(t, pkg)
				require.NotNil(t, pkg.Data)
				require.NotNil(t, pkg.Data.ImaMeasurementList)
				require.Empty(t, *pkg.Data.ImaMeasurementList)
				require.NotNil(t, pkg.Data.MbLog)
				require.Empty(t, *pkg.Data.MbLog)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAttestationData(tt.enrollmentRequest, logger)

			if tt.expectNil {
				require.Nil(t, result)
			} else {
				require.NotNil(t, result)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

func TestGetStringValue(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected string
	}{
		{
			name:     "non-nil string",
			input:    lo.ToPtr("test-value"),
			expected: "test-value",
		},
		{
			name:     "nil string",
			input:    nil,
			expected: "",
		},
		{
			name:     "empty string",
			input:    lo.ToPtr(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringValue(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
