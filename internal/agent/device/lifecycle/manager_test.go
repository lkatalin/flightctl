package lifecycle

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/flightctl/flightctl/api/core/v1beta1"
	"github.com/flightctl/flightctl/internal/agent/client"
	"github.com/flightctl/flightctl/internal/agent/device/fileio"
	"github.com/flightctl/flightctl/internal/agent/device/status"
	"github.com/flightctl/flightctl/internal/agent/identity"
	"github.com/flightctl/flightctl/pkg/log"
	"github.com/google/go-tpm/tpm2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/util/wait"
)

// generateTestCertificate creates a valid test certificate for testing
func generateTestCertificate(t *testing.T) string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-device",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour * 24 * 365),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: nil,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return string(certPEM)
}

func TestLifecycleManager_verifyEnrollment(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(*client.MockEnrollment, *identity.MockProvider, *fileio.MockReadWriter)
		expectedResult bool
		expectedError  string
	}{
		{
			name: "identity proof required and succeeds",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							// No "Approved" condition, so identity proof is required
						},
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
				mockIdentity.EXPECT().ProveIdentity(gomock.Any(), enrollmentRequest).Return(nil)
			},
			expectedResult: false,
			expectedError:  "",
		},
		{
			name: "identity proof fails with ErrIdentityProofFailed",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							// No "Approved" condition, so identity proof is required
						},
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
				mockIdentity.EXPECT().ProveIdentity(gomock.Any(), enrollmentRequest).Return(identity.ErrIdentityProofFailed)
			},
			expectedResult: false,
			expectedError:  "proving identity: identity proof failed",
		},
		{
			name: "identity proof fails with other error",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							// No "Approved" condition, so identity proof is required
						},
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
				mockIdentity.EXPECT().ProveIdentity(gomock.Any(), enrollmentRequest).Return(errors.New("network error"))
			},
			expectedResult: false,
			expectedError:  "",
		},
		{
			name: "enrollment denied",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							{
								Type:    "Denied",
								Reason:  "PolicyViolation",
								Message: "Device does not meet policy requirements",
							},
						},
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
			},
			expectedResult: false,
			expectedError:  "enrollment request denied: reason: PolicyViolation, message: Device does not meet policy requirements",
		},
		{
			name: "enrollment failed",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							{
								Type:    "Failed",
								Reason:  "ProcessingError",
								Message: "Failed to process enrollment request",
							},
						},
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
			},
			expectedResult: false,
			expectedError:  "enrollment request failed: reason: ProcessingError, message: Failed to process enrollment request",
		},
		{
			name: "enrollment approved with certificate",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				certificate := generateTestCertificate(t)
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							{
								Type: "Approved",
							},
						},
						Certificate: &certificate,
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
				mockIdentity.EXPECT().StoreCertificate([]byte(certificate)).Return(nil)
				// CSR cleanup now uses standalone functions (identity.LoadCSR/StoreCSR) with mockReadWriter
				mockReadWriter.EXPECT().PathExists("certs/agent.csr").Return(true, nil)
				mockReadWriter.EXPECT().ReadFile("certs/agent.csr").Return([]byte("test-csr"), nil)
				mockReadWriter.EXPECT().PathExists("certs/agent.csr").Return(true, nil)
				mockReadWriter.EXPECT().OverwriteAndWipe("certs/agent.csr").Return(nil)
			},
			expectedResult: true,
			expectedError:  "",
		},
		{
			name: "enrollment approved but no certificate yet",
			setupMocks: func(mockEnrollment *client.MockEnrollment, mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				enrollmentRequest := &v1beta1.EnrollmentRequest{
					Status: &v1beta1.EnrollmentRequestStatus{
						Conditions: []v1beta1.Condition{
							{
								Type: "Approved",
							},
						},
						Certificate: nil,
					},
				}
				mockEnrollment.EXPECT().GetEnrollmentRequest(gomock.Any(), "test-device").Return(enrollmentRequest, nil)
			},
			expectedResult: false,
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockEnrollment := client.NewMockEnrollment(ctrl)
			mockIdentity := identity.NewMockProvider(ctrl)
			mockStatus := status.NewMockManager(ctrl)
			mockReadWriter := fileio.NewMockReadWriter(ctrl)

			manager := &LifecycleManager{
				deviceName:       "test-device",
				enrollmentClient: mockEnrollment,
				identityProvider: mockIdentity,
				statusManager:    mockStatus,
				deviceReadWriter: mockReadWriter,
				backoff:          wait.Backoff{},
				log:              log.NewPrefixLogger("test"),
			}

			tt.setupMocks(mockEnrollment, mockIdentity, mockReadWriter)

			ctx := context.Background()
			result, err := manager.verifyEnrollment(ctx)

			require.Equal(t, tt.expectedResult, result)
			if tt.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestLifecycleManager_collectAttestationData(t *testing.T) {
	tests := []struct {
		name               string
		setupMocks         func(*identity.MockProvider, *fileio.MockReadWriter)
		expectError        bool
		expectedErrMessage string
		validateResult     func(*testing.T, *v1beta1.AttestationData)
	}{
		{
			name: "successful attestation data collection",
			setupMocks: func(mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				// Mock TPM client
				tpmClient := &mockTPMClient{
					quote:     []byte("test-quote"),
					signature: []byte("test-signature"),
					pcrs:      []byte("test-pcrs"),
					akPublic:  []byte("test-ak-public"),
					ekPublic:  []byte("test-ek-public"),
				}

				mockIdentity.EXPECT().GetTPMClient().Return(tpmClient, nil)

				// Mock IMA measurements (optional)
				mockReadWriter.EXPECT().PathExists("/sys/kernel/security/ima/ascii_runtime_measurements").Return(true, nil)
				mockReadWriter.EXPECT().ReadFile("/sys/kernel/security/ima/ascii_runtime_measurements").Return([]byte("10 hash ima-ng sha256:abc /bin/ls\n"), nil)

				// Mock measured boot log (optional)
				mockReadWriter.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(true, nil)
				mockReadWriter.EXPECT().ReadFile("/sys/kernel/security/tpm0/binary_bios_measurements").Return([]byte{0x00, 0x01, 0x02}, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, data *v1beta1.AttestationData) {
				require.NotNil(t, data)
				require.NotNil(t, data.Quote)
				require.NotEmpty(t, *data.Quote)
				require.NotNil(t, data.Nonce)
				require.NotEmpty(t, *data.Nonce)
				require.NotNil(t, data.HashAlg)
				require.Equal(t, "sha256", *data.HashAlg)
				require.NotNil(t, data.TpmAk)
				require.NotEmpty(t, *data.TpmAk)
				require.NotNil(t, data.TpmEk)
				require.NotEmpty(t, *data.TpmEk)
				require.NotNil(t, data.ImaMeasurementList)
				require.NotEmpty(t, *data.ImaMeasurementList)
				require.NotNil(t, data.MbLog)
				require.NotEmpty(t, *data.MbLog)
			},
		},
		{
			name: "TPM client not available",
			setupMocks: func(mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				mockIdentity.EXPECT().GetTPMClient().Return(nil, errors.New("TPM client not initialized"))
			},
			expectError:        true,
			expectedErrMessage: "getting TPM client",
		},
		{
			name: "quote generation fails",
			setupMocks: func(mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				tpmClient := &mockTPMClient{
					quoteErr: errors.New("TPM quote failed"),
				}
				mockIdentity.EXPECT().GetTPMClient().Return(tpmClient, nil)
			},
			expectError:        true,
			expectedErrMessage: "generating TPM quote",
		},
		{
			name: "successful with missing optional measurements",
			setupMocks: func(mockIdentity *identity.MockProvider, mockReadWriter *fileio.MockReadWriter) {
				tpmClient := &mockTPMClient{
					quote:     []byte("test-quote"),
					signature: []byte("test-signature"),
					pcrs:      []byte("test-pcrs"),
					akPublic:  []byte("test-ak-public"),
					ekPublic:  []byte("test-ek-public"),
				}

				mockIdentity.EXPECT().GetTPMClient().Return(tpmClient, nil)

				// IMA not available
				mockReadWriter.EXPECT().PathExists("/sys/kernel/security/ima/ascii_runtime_measurements").Return(false, nil)

				// Measured boot not available
				mockReadWriter.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(false, nil)
				mockReadWriter.EXPECT().PathExists("/sys/kernel/security/tpm0/binary_bios_measurements").Return(false, nil)
				mockReadWriter.EXPECT().PathExists("/sys/class/tpm/tpm0/device/binary_bios_measurements").Return(false, nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, data *v1beta1.AttestationData) {
				require.NotNil(t, data)
				require.NotNil(t, data.Quote)
				require.NotEmpty(t, *data.Quote)
				// IMA and MB logs should be nil when not available
				if data.ImaMeasurementList != nil {
					require.Empty(t, *data.ImaMeasurementList)
				}
				require.Nil(t, data.MbLog)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockIdentity := identity.NewMockProvider(ctrl)
			mockReadWriter := fileio.NewMockReadWriter(ctrl)

			manager := &LifecycleManager{
				identityProvider: mockIdentity,
				deviceReadWriter: mockReadWriter,
				log:              log.NewPrefixLogger("test"),
			}

			tt.setupMocks(mockIdentity, mockReadWriter)

			ctx := context.Background()
			result, err := manager.collectAttestationData(ctx)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrMessage != "" {
					require.Contains(t, err.Error(), tt.expectedErrMessage)
				}
			} else {
				require.NoError(t, err)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

// mockTPMClient is a simple mock for testing attestation data collection
type mockTPMClient struct {
	quote     []byte
	signature []byte
	pcrs      []byte
	akPublic  []byte
	ekPublic  []byte
	quoteErr  error
	akErr     error
	ekErr     error
}

func (m *mockTPMClient) GenerateQuote(nonce []byte, pcrSelection *tpm2.TPMLPCRSelection) ([]byte, []byte, []byte, error) {
	if m.quoteErr != nil {
		return nil, nil, nil, m.quoteErr
	}
	return m.quote, m.signature, m.pcrs, nil
}

func (m *mockTPMClient) GetAKPublic() ([]byte, error) {
	if m.akErr != nil {
		return nil, m.akErr
	}
	return m.akPublic, nil
}

func (m *mockTPMClient) GetEKPublic() ([]byte, error) {
	if m.ekErr != nil {
		return nil, m.ekErr
	}
	return m.ekPublic, nil
}

func (m *mockTPMClient) GetHashAlgorithm() string {
	return "sha256"
}

// Implement other TPM client methods as no-ops for interface compliance
func (m *mockTPMClient) Close() error {
	return nil
}

func (m *mockTPMClient) Public() crypto.PublicKey {
	return nil
}

func (m *mockTPMClient) GetSigner() crypto.Signer {
	return nil
}

func (m *mockTPMClient) MakeCSR(deviceName string, qualifyingData []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockTPMClient) SolveChallenge(credentialBlob, encryptedSecret []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockTPMClient) CreateApplicationKey(name string) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (m *mockTPMClient) UpdateNonce(nonce []byte) error {
	return nil
}

func (m *mockTPMClient) Clear() error {
	return nil
}

func (m *mockTPMClient) VendorInfoCollector(ctx context.Context) string {
	return ""
}
