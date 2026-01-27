package lifecycle

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flightctl/flightctl/api/core/v1beta1"
	"github.com/flightctl/flightctl/internal/agent/client"
	"github.com/flightctl/flightctl/internal/agent/device/errors"
	"github.com/flightctl/flightctl/internal/agent/device/fileio"
	"github.com/flightctl/flightctl/internal/agent/device/status"
	"github.com/flightctl/flightctl/internal/agent/identity"
	"github.com/flightctl/flightctl/internal/tpm"
	"github.com/flightctl/flightctl/pkg/log"
	"github.com/skip2/go-qrcode"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/cert"
)

const (
	// agent banner file
	BannerFile           = "/etc/issue.d/flightctl-banner.issue"
	identityProofTimeout = 60 * time.Second
)

var (
	_ Manager     = (*LifecycleManager)(nil)
	_ Initializer = (*LifecycleManager)(nil)
)

// LifecycleManager struct needs to hold a reference to the management config
type LifecycleManager struct {
	deviceName           string
	enrollmentUIEndpoint string
	managementCertPath   string
	managementKeyPath    string
	dataDir              string
	deviceReadWriter     fileio.ReadWriter

	enrollmentClient client.Enrollment
	defaultLabels    map[string]string
	enrollmentCSR    []byte
	statusManager    status.Manager
	systemdClient    *client.Systemd
	identityProvider identity.Provider

	backoff wait.Backoff
	log     *log.PrefixLogger
}

// Manager is responsible for managing the device lifecycle.
func NewManager(
	deviceName string,
	enrollmentUIEndpoint string,
	managementCertPath string,
	managementKeyPath string,
	dataDir string,
	deviceReadWriter fileio.ReadWriter,
	enrollmentClient client.Enrollment,
	enrollmentCSR []byte,
	defaultLabels map[string]string,
	statusManager status.Manager,
	systemdClient *client.Systemd,
	identityProvider identity.Provider,
	backoff wait.Backoff,
	log *log.PrefixLogger,
) *LifecycleManager {
	return &LifecycleManager{
		log:                  log,
		deviceName:           deviceName,
		enrollmentUIEndpoint: enrollmentUIEndpoint,
		managementCertPath:   managementCertPath,
		managementKeyPath:    managementKeyPath,
		dataDir:              dataDir,
		deviceReadWriter:     deviceReadWriter,
		enrollmentClient:     enrollmentClient,
		enrollmentCSR:        enrollmentCSR,
		defaultLabels:        defaultLabels,
		backoff:              backoff,
		statusManager:        statusManager,
		systemdClient:        systemdClient,
		identityProvider:     identityProvider,
	}
}

// Initialize ensures the device is enrolled to the management service.
func (m *LifecycleManager) Initialize(ctx context.Context, status *v1beta1.DeviceStatus) error {
	if !m.IsInitialized() {
		if err := m.writeEnrollmentBanner(ctx); err != nil {
			return err
		}

		if err := m.enrollmentRequest(ctx, status); err != nil {
			return err
		}

		m.log.Info("Waiting for enrollment to be approved")
		err := wait.ExponentialBackoffWithContext(ctx, m.backoff, func(ctx context.Context) (bool, error) {
			return m.verifyEnrollment(ctx)
		})
		if err != nil {
			return err
		}
	}

	// write the management banner
	return m.writeManagementBanner(ctx)
}

func (m *LifecycleManager) Sync(ctx context.Context, current, desired *v1beta1.DeviceSpec) error {
	// this controller currently does not implement a sync operation
	return nil
}

func (m *LifecycleManager) AfterUpdate(ctx context.Context, current, desired *v1beta1.DeviceSpec) error {
	var errs []error
	if current.Decommissioning == nil && desired.Decommissioning != nil {
		m.log.Warn("Detected decommissioning request from flightctl service")
		m.log.Warn("Updating Condition to decommissioning started")
		if err := m.updateWithStartedCondition(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%w started Condition: %w", errors.ErrFailedToUpdateStatusWithDecommission, err))
			m.log.Warn("Unable to update Condition to decommissioning started")
		}

		// TODO: add support for additional decommissioning target types.
		// these are the steps that will take places between Started and Completed status

		if len(errs) == 0 {
			m.log.Warn("No errors during decommissioning prior to wiping key and cert; updating Condition to decommissioning completed")
			if err := m.updateWithCompletedCondition(ctx); err != nil {
				errs = append(errs, fmt.Errorf("%w completed Condition: %w", errors.ErrFailedToUpdateStatusWithDecommission, err))
				m.log.Warn("Unable to update Condition to decommissioning completed")
			}
		} else {
			m.log.Warn("Errors encountered during decommissioning; updating Condition to decommission error")
			if err := m.updateWithErrorCondition(ctx, errs); err != nil {
				errs = append(errs, fmt.Errorf("%w errored Condition: %w", errors.ErrFailedToUpdateStatusWithDecommission, err))
				m.log.Warn("Unable to update Condition to decommissioning error")
			}
		}

		// after this point the device will no longer be able to communicate with the management service
		m.log.Warn("Preparing to wipe agent certificate and keys and reboot")
		if err := m.wipeAndReboot(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *LifecycleManager) updateWithStartedCondition(ctx context.Context) error {
	updateErr := m.statusManager.UpdateCondition(ctx, v1beta1.Condition{
		Type:    v1beta1.ConditionTypeDeviceDecommissioning,
		Status:  v1beta1.ConditionStatusTrue,
		Reason:  string(v1beta1.DecommissionStateStarted),
		Message: "Device started decommissioning",
	})
	if updateErr != nil {
		m.log.Warnf("Failed setting status: %v", updateErr)
		return fmt.Errorf("failed to update decommission started status: %w", updateErr)
	}
	return nil
}

func (m *LifecycleManager) updateWithCompletedCondition(ctx context.Context) error {
	updateErr := m.statusManager.UpdateCondition(ctx, v1beta1.Condition{
		Type:    v1beta1.ConditionTypeDeviceDecommissioning,
		Status:  v1beta1.ConditionStatusTrue,
		Reason:  string(v1beta1.DecommissionStateComplete),
		Message: "Device completed decommissioning and will wipe its management certificate",
	})
	if updateErr != nil {
		m.log.Warnf("Failed setting status: %v", updateErr)
		return fmt.Errorf("failed to update decommission completed status: %w", updateErr)
	}
	return nil
}

func (m *LifecycleManager) updateWithErrorCondition(ctx context.Context, errs []error) error {
	updateErr := m.statusManager.UpdateCondition(ctx, v1beta1.Condition{
		Type:    v1beta1.ConditionTypeDeviceDecommissioning,
		Status:  v1beta1.ConditionStatusTrue,
		Reason:  string(v1beta1.DecommissionStateError),
		Message: fmt.Sprintf("Device encountered one or more errors during decommissioning: %v", errors.Join(errs...)),
	})
	if updateErr != nil {
		m.log.Warnf("Failed setting status: %v", updateErr)
		return fmt.Errorf("failed to update decommission errored status: %w", updateErr)
	}
	return nil
}

// point of no return - wipes management cert and keys
func (m *LifecycleManager) wipeAndReboot(ctx context.Context) error {
	var errs []error

	// Use identity provider to wipe credentials securely
	if err := m.identityProvider.WipeCredentials(); err != nil {
		m.log.Errorf("Failed to wipe credentials via identity provider: %v", err)
		errs = append(errs, fmt.Errorf("failed to wipe credentials via identity provider: %w", err))
	}

	// Clear sensitive data ahead of time in case reboot fails
	m.deviceName = ""
	m.enrollmentUIEndpoint = ""
	m.enrollmentClient = nil
	m.enrollmentCSR = nil
	// delete desired.json current.json rollback.json
	errs = m.deleteSpec(errs)

	// TODO: incorporate before-reboot hooks
	if err := m.systemdClient.Reboot(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to initiate system reboot: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *LifecycleManager) deleteSpec(errs []error) []error {
	if err := m.deviceReadWriter.RemoveFile(filepath.Join(m.dataDir, "desired.json")); err != nil {
		m.log.Errorf("Failed to delete desired.json: %v", err)
		errs = append(errs, fmt.Errorf("failed to delete desired.json: %w", err))
	}
	if err := m.deviceReadWriter.RemoveFile(filepath.Join(m.dataDir, "current.json")); err != nil {
		m.log.Errorf("Failed to delete current.json: %v", err)
		errs = append(errs, fmt.Errorf("failed to delete current.json: %w", err))
	}
	if err := m.deviceReadWriter.RemoveFile(filepath.Join(m.dataDir, "rollback.json")); err != nil {
		m.log.Errorf("Failed to delete rollback.json: %v", err)
		errs = append(errs, fmt.Errorf("failed to delete rollback.json: %w", err))
	}
	return errs
}

func (m *LifecycleManager) IsInitialized() bool {
	// check if the identity provider has a certificate
	return m.identityProvider.HasCertificate()
}

func (m *LifecycleManager) verifyEnrollment(ctx context.Context) (bool, error) {
	enrollmentRequest, err := m.enrollmentClient.GetEnrollmentRequest(ctx, m.deviceName)
	if err != nil {
		m.log.Errorf("Error checking enrollment status: %v", err)
		return false, nil
	}

	// TODO: update schema to require condition in status, then remove this check
	if enrollmentRequest.Status == nil || enrollmentRequest.Status.Conditions == nil {
		return false, fmt.Errorf("enrollment request status or conditions field are nil")
	}

	approved := false
	for _, cond := range enrollmentRequest.Status.Conditions {
		if cond.Type == "Denied" {
			return false, fmt.Errorf("%w: reason: %v, message: %v", errors.ErrEnrollmentRequestDenied, cond.Reason, cond.Message)
		}
		if cond.Type == "Failed" {
			return false, fmt.Errorf("%w: reason: %v, message: %v", errors.ErrEnrollmentRequestFailed, cond.Reason, cond.Message)
		}
		if cond.Type == "Approved" {
			approved = true
		}
	}
	if !approved {
		// While pending approval, take the time to verify the identity. The provider
		// is responsible for determining whether proof is required
		ctx, cancel := context.WithTimeout(ctx, identityProofTimeout)
		defer cancel()
		if err := m.identityProvider.ProveIdentity(ctx, enrollmentRequest); err != nil {
			if errors.Is(err, identity.ErrIdentityProofFailed) {
				return false, fmt.Errorf("proving identity: %w", err)
			}
			m.log.Warnf("A retryable error occurred while proving the agent's identity: %v", err)
			return false, nil
		}
		m.log.Info("Enrollment request not yet approved")
		return false, nil
	}
	if enrollmentRequest.Status.Certificate == nil {
		m.log.Infof("Enrollment request approved, but certificate not yet issued")
		return false, nil
	}
	if len(*enrollmentRequest.Status.Certificate) == 0 {
		m.log.Infof("Enrollment request approved, but certificate not yet issued")
		return false, nil
	}
	m.log.Infof("Enrollment approved and certificate issued")

	if _, err = cert.ParseCertsPEM([]byte(*enrollmentRequest.Status.Certificate)); err != nil {
		return false, fmt.Errorf("parsing signed certificate: %v", err)
	}

	if err := m.identityProvider.StoreCertificate([]byte(*enrollmentRequest.Status.Certificate)); err != nil {
		return false, fmt.Errorf("failed to store certificate: %w", err)
	}

	// Clear the persisted CSR once certificate is obtained
	clientCSRPath := identity.GetCSRPath(m.dataDir)
	if _, found, err := identity.LoadCSR(m.deviceReadWriter, clientCSRPath); err == nil && found {
		m.log.Infof("Clearing persisted CSR after successful enrollment")
		if err := identity.StoreCSR(m.deviceReadWriter, clientCSRPath, nil); err != nil {
			m.log.Warnf("Failed to clear persisted CSR: %v", err)
		}
	}

	return true, nil
}

func (m *LifecycleManager) writeEnrollmentBanner(ctx context.Context) error {
	if m.enrollmentUIEndpoint == "" {
		m.log.Warn("Flightctl enrollment UI endpoint is missing, skipping enrollment banner")
		return nil
	}
	url := fmt.Sprintf("%s/enroll/%s", m.enrollmentUIEndpoint, m.deviceName)
	if err := m.writeQRBanner(ctx, "\nEnroll your device to flightctl by scanning\nthe above QR code or following this URL:\n%s\n\n", url); err != nil {
		return fmt.Errorf("failed to write device enrollment banner: %w", err)
	}
	return nil
}

func (m *LifecycleManager) writeManagementBanner(ctx context.Context) error {
	// write a banner that explains that the device is enrolled
	if m.enrollmentUIEndpoint == "" {
		m.log.Warn("Flightctl enrollment UI endpoint is missing, skipping management banner")
		return nil
	}
	url := fmt.Sprintf("%s/manage/%s", m.enrollmentUIEndpoint, m.deviceName)
	if err := m.writeQRBanner(ctx, "\nYour device is enrolled to flightctl,\nyou can manage your device scanning the above QR. or following this URL:\n%s\n\n", url); err != nil {
		return fmt.Errorf("failed to write device management banner: %w", err)
	}
	return nil
}

func (m *LifecycleManager) writeQRBanner(ctx context.Context, message, url string) error {
	qrCode, err := qrcode.New(url, qrcode.High)
	if err != nil {
		return fmt.Errorf("failed to generate new QR code: %w", err)
	}

	// convert the QR code to a string.
	qrString := qrCode.ToSmallString(false)

	// write a banner that explains that the device is enrolled
	buffer := bytes.NewBufferString("\n")
	buffer.WriteString(qrString)

	// write the QR code to the buffer
	fmt.Fprintf(buffer, message, url)

	// duplicate file to /etc/issue.d/flightctl-banner.issue
	if err := m.deviceReadWriter.WriteFile(BannerFile, buffer.Bytes(), fileio.DefaultFilePermissions); err != nil {
		return fmt.Errorf("failed to write banner to disk: %w", err)
	}

	if err := m.systemdClient.SdNotify(ctx, "READY=1"); err != nil {
		m.log.Warnf("Failed to notify systemd: %v", err)
	}

	value := os.Getenv("FLIGHTCTL_DISABLE_CONSOLE_BANNER")
	if !(strings.EqualFold(value, "true") || value == "1") {
		fmt.Println(buffer.String())
	}
	return nil
}

func (m *LifecycleManager) enrollmentRequest(ctx context.Context, deviceStatus *v1beta1.DeviceStatus) error {
	var csrString string
	if tpm.IsTCGCSRFormat(m.enrollmentCSR) {
		// TCG CSR is binary data, must be base64 encoded
		m.log.Debugf("Detected TCG CSR format, base64 encoding before transmission")
		csrString = base64.StdEncoding.EncodeToString(m.enrollmentCSR)
	} else {
		csrString = string(m.enrollmentCSR)
	}

	// Try to read knownRenderedVersion from desired.json
	var knownRenderedVersion *string
	desiredPath := filepath.Join(m.dataDir, "desired.json")
	if desiredBytes, err := m.deviceReadWriter.ReadFile(desiredPath); err == nil {
		var desired v1beta1.Device
		if err := json.Unmarshal(desiredBytes, &desired); err == nil {
			if version := desired.Version(); version != "" {
				knownRenderedVersion = &version
				m.log.Debugf("Found knownRenderedVersion from desired.json: %s", version)
			}
		} else {
			m.log.Debugf("Failed to unmarshal desired.json: %v", err)
		}
	} else {
		m.log.Debugf("Failed to read desired.json: %v", err)
	}

	// Collect attestation data if enabled
	var attestationData *v1beta1.AttestationData
	if m.identityProvider.IsAttestationEnabled() {
		m.log.Info("Attestation enabled, collecting TPM quote and measurements")
		var err error
		attestationData, err = m.collectAttestationData(ctx)
		if err != nil {
			m.log.Warnf("Failed to collect attestation data: %v", err)
			// Continue without attestation data if collection fails
			attestationData = nil
		}
	}

	req := v1beta1.EnrollmentRequest{
		ApiVersion: "v1beta1",
		Kind:       "EnrollmentRequest",
		Metadata: v1beta1.ObjectMeta{
			Name: &m.deviceName,
		},
		Spec: v1beta1.EnrollmentRequestSpec{
			Csr:                  csrString,
			DeviceStatus:         deviceStatus,
			Labels:               &m.defaultLabels,
			KnownRenderedVersion: knownRenderedVersion,
			AttestationData:      attestationData,
		},
	}

	err := wait.ExponentialBackoffWithContext(ctx, m.backoff, func(ctx context.Context) (bool, error) {
		_, err := m.enrollmentClient.CreateEnrollmentRequest(ctx, req)
		if err != nil {
			m.log.Warnf("failed to create enrollment request: %v", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("creating enrollment request: %w", err)
	}

	return nil
}

// collectAttestationData collects TPM attestation data for enrollment
func (m *LifecycleManager) collectAttestationData(ctx context.Context) (*v1beta1.AttestationData, error) {
	tpmClient, err := m.identityProvider.GetTPMClient()
	if err != nil {
		return nil, fmt.Errorf("getting TPM client: %w", err)
	}
	if tpmClient == nil {
		return nil, fmt.Errorf("TPM client is nil")
	}

	// Generate a nonce for the quote (20 bytes as per Keylime spec)
	nonce := make([]byte, 20)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Generate TPM quote with PCR values
	quote, signature, pcrs, err := tpmClient.GenerateQuote(nonce, nil)
	if err != nil {
		return nil, fmt.Errorf("generating TPM quote: %w", err)
	}

	// Get TPM keys
	akPublic, err := tpmClient.GetAKPublic()
	if err != nil {
		return nil, fmt.Errorf("getting AK public key: %w", err)
	}

	ekPublic, err := tpmClient.GetEKPublic()
	if err != nil {
		return nil, fmt.Errorf("getting EK public key: %w", err)
	}

	// Combine quote, signature, and PCRs for transmission
	// Format: base64(quote:signature:pcrs)
	combinedQuote := fmt.Sprintf("%s:%s:%s",
		base64.StdEncoding.EncodeToString(quote),
		base64.StdEncoding.EncodeToString(signature),
		base64.StdEncoding.EncodeToString(pcrs))

	// Read IMA measurements if available
	imaMeasurements, err := tpm.ReadIMAMeasurements(m.deviceReadWriter)
	if err != nil {
		m.log.Warnf("Failed to read IMA measurements: %v", err)
		imaMeasurements = ""
	}

	// Read measured boot log if available
	mbLog, err := tpm.ReadMeasuredBootLog(m.deviceReadWriter)
	if err != nil {
		m.log.Warnf("Failed to read measured boot log: %v", err)
		mbLog = nil
	}

	// Prepare attestation data
	hashAlg := tpmClient.GetHashAlgorithm()
	nonceStr := base64.StdEncoding.EncodeToString(nonce)
	akStr := base64.StdEncoding.EncodeToString(akPublic)
	ekStr := base64.StdEncoding.EncodeToString(ekPublic)

	attestation := &v1beta1.AttestationData{
		Quote:   &combinedQuote,
		Nonce:   &nonceStr,
		HashAlg: &hashAlg,
		TpmAk:   &akStr,
		TpmEk:   &ekStr,
	}

	if imaMeasurements != "" {
		attestation.ImaMeasurementList = &imaMeasurements
	}

	if mbLog != nil {
		mbLogStr := base64.StdEncoding.EncodeToString(mbLog)
		attestation.MbLog = &mbLogStr
	}

	m.log.Infof("Collected attestation data: quote=%d bytes, AK=%d bytes, EK=%d bytes, IMA=%d bytes, MB=%d bytes",
		len(combinedQuote), len(akStr), len(ekStr), len(imaMeasurements), len(mbLog))

	return attestation, nil
}
