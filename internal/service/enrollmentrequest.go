package service

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flightctl/flightctl/internal/config/ca"
	"github.com/flightctl/flightctl/internal/consts"
	"github.com/flightctl/flightctl/internal/contextutil"
	"github.com/flightctl/flightctl/internal/crypto"
	"github.com/flightctl/flightctl/internal/crypto/signer"
	"github.com/flightctl/flightctl/internal/domain"
	"github.com/flightctl/flightctl/internal/flterrors"
	"github.com/flightctl/flightctl/internal/kvstore"
	"github.com/flightctl/flightctl/internal/service/common"
	"github.com/flightctl/flightctl/internal/store/selector"
	"github.com/flightctl/flightctl/internal/tpm"
	"github.com/flightctl/flightctl/internal/util"
	"github.com/google/go-tpm/tpm2"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// stripTPM2BPrefix removes the 2-byte TPM2B length prefix from a base64-encoded blob
// TPM2B structures have a 2-byte length field, but Keylime expects raw structures without this wrapper
func stripTPM2BPrefix(base64Blob string) string {
	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(base64Blob)
	if err != nil || len(data) < 2 {
		// If decode fails or data is too short, return original
		return base64Blob
	}

	// Strip first 2 bytes (TPM2B length prefix)
	strippedData := data[2:]

	// Re-encode to base64
	return base64.StdEncoding.EncodeToString(strippedData)
}

// parsePCRSelectionFromQuote extracts the PCR selection bitmask from the TPM quote
// The quote format is "quote:signature:pcr" where pcr contains TPML_PCR_SELECTION structure
// Returns the list of PCR numbers that are included in the quote
func parsePCRSelectionFromQuote(quoteParts []string) ([]int, error) {
	if len(quoteParts) != 3 {
		return nil, fmt.Errorf("invalid quote format, expected 3 parts")
	}

	// Decode the PCR data (third part)
	pcrData, err := base64.StdEncoding.DecodeString(quoteParts[2])
	if err != nil {
		return nil, fmt.Errorf("failed to decode PCR data: %w", err)
	}

	// Parse the Intel format PCR structure
	// Format: count (4 bytes LE), then 16 TPMS_PCR_SELECTION entries
	// Each entry: hash_alg (2 bytes LE), size_of_select (1 byte), pcr_select (3 bytes), padding (2 bytes)
	if len(pcrData) < 4 {
		return nil, fmt.Errorf("PCR data too short")
	}

	// Read count (first 4 bytes, little-endian)
	count := uint32(pcrData[0]) | uint32(pcrData[1])<<8 | uint32(pcrData[2])<<16 | uint32(pcrData[3])<<24
	if count == 0 {
		return nil, nil // No PCR selections
	}

	pcrs := make([]int, 0)
	offset := 4 // Skip count field

	// Parse first PCR selection (we expect SHA256 bank)
	// Each entry is 8 bytes: hash_alg (2), size_of_select (1), pcr_select (3), padding (2)
	if len(pcrData) < offset+8 {
		return nil, fmt.Errorf("PCR data too short for selection entry")
	}

	// Read hash algorithm (2 bytes, little-endian)
	// hashAlg := uint16(pcrData[offset]) | uint16(pcrData[offset+1])<<8

	// Read size of select (1 byte)
	sizeOfSelect := int(pcrData[offset+2])

	// Read PCR select bitmask (3 bytes)
	if sizeOfSelect >= 3 && len(pcrData) >= offset+6 {
		pcrSelect := [3]byte{pcrData[offset+3], pcrData[offset+4], pcrData[offset+5]}

		// Extract PCR numbers from bitmask
		for pcrNum := 0; pcrNum < 24; pcrNum++ {
			byteIdx := pcrNum / 8
			bitIdx := pcrNum % 8
			if (pcrSelect[byteIdx] & (1 << bitIdx)) != 0 {
				pcrs = append(pcrs, pcrNum)
			}
		}
	}

	return pcrs, nil
}

// buildTPMPolicyFromPCRs constructs a TPM policy JSON with appropriate mask for the given PCRs
func buildTPMPolicyFromPCRs(pcrs []int, existingPolicy *string) (string, error) {
	// If there's an existing policy, try to merge with it
	var policyMap map[string]interface{}
	if existingPolicy != nil && *existingPolicy != "" {
		if err := json.Unmarshal([]byte(*existingPolicy), &policyMap); err != nil {
			// If existing policy is invalid, start fresh
			policyMap = make(map[string]interface{})
		}
	} else {
		policyMap = make(map[string]interface{})
	}

	// Calculate mask based on PCRs present in the quote
	// Mask is a hex string where each bit represents a PCR
	if len(pcrs) == 0 {
		// No PCRs in quote - use mask 0x0
		policyMap["mask"] = "0x0"
	} else {
		// Calculate mask value from PCR list
		mask := uint32(0)
		for _, pcrNum := range pcrs {
			if pcrNum >= 0 && pcrNum < 32 {
				mask |= (1 << uint(pcrNum))
			}
		}
		policyMap["mask"] = fmt.Sprintf("0x%x", mask)
	}

	// Marshal back to JSON
	policyJSON, err := json.Marshal(policyMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal TPM policy: %w", err)
	}

	return string(policyJSON), nil
}

// convertTPM2BPublicToPEM converts a base64-encoded TPM2B_PUBLIC to PEM format
// Keylime expects the AK in PEM format, not raw TPM2B_PUBLIC format
func convertTPM2BPublicToPEM(base64TPM2BPublic string) (string, error) {
	// Decode base64 to get TPM2B_PUBLIC bytes
	tpm2bBytes, err := base64.StdEncoding.DecodeString(base64TPM2BPublic)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 TPM2B_PUBLIC: %w", err)
	}

	// Unmarshal TPM2B_PUBLIC
	tpm2bPublic, err := tpm2.Unmarshal[tpm2.TPM2BPublic](tpm2bBytes)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal TPM2B_PUBLIC: %w", err)
	}

	// Convert TPM2B_PUBLIC to crypto.PublicKey
	pubKey, err := tpm.ConvertTPM2BPublicToPublicKey(tpm2bPublic)
	if err != nil {
		return "", fmt.Errorf("failed to convert TPM2B_PUBLIC to crypto.PublicKey: %w", err)
	}

	// Marshal to PKIX format (standard public key format)
	pkixBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key to PKIX: %w", err)
	}

	// Encode to PEM format
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pkixBytes,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	return string(pemBytes), nil
}

// getTPMCAPool loads the TPM CA certificates from configured paths
func (h *ServiceHandler) getTPMCAPool() *x509.CertPool {
	if len(h.tpmCAPaths) == 0 {
		return nil
	}

	roots, err := tpm.LoadCAsFromPaths(h.tpmCAPaths)
	if err != nil {
		h.log.Warnf("Failed to load TPM CA certificates from configured paths: %v", err)
		return nil
	}

	return roots
}

func (h *ServiceHandler) verifyTPMEnrollmentRequest(er *domain.EnrollmentRequest, name string) error {
	csrBytes, isTPM := tpm.ParseTCGCSRBytes(er.Spec.Csr)
	if !isTPM {
		return fmt.Errorf("failed to parse TCG CSR")
	}

	trustedRoots := h.getTPMCAPool()
	condition := domain.Condition{
		Type:   domain.ConditionTypeEnrollmentRequestTPMVerified,
		Status: domain.ConditionStatusFalse,
	}
	if err := tpm.VerifyTCGCSRChainOfTrustWithRoots(csrBytes, trustedRoots); err != nil {
		condition.Reason = domain.TPMVerificationFailedReason
		condition.Message = err.Error()
		h.log.Warnf("TPM verification failed for enrollment request %s: %v", name, err)
	} else {
		condition.Reason = domain.TPMChallengeRequiredReason
		condition.Message = "TPM chain of trust partially verified, activate credential challenge required"
		h.log.Debugf("TPM chain partially verified for enrollment request %s, challenge required", name)
	}
	domain.SetStatusCondition(&er.Status.Conditions, condition)
	return nil
}

// processAttestationWithKeylime sends attestation data to Keylime verifier and updates enrollment request status
func (h *ServiceHandler) processAttestationWithKeylime(ctx context.Context, orgId uuid.UUID, attestationPkg *AttestationPackage, er *domain.EnrollmentRequest) error {
	// Skip if Keylime client is not configured (MOCK MODE)
	if h.keylimeClient == nil {
		h.log.Warnf("⚠️  ATTESTATION MOCK MODE: Keylime verifier not enabled - skipping TPM attestation verification for device %s. THIS IS INSECURE AND SHOULD NOT BE USED IN PRODUCTION!", attestationPkg.Metadata.EnrollmentRequestName)
		// Add a condition to the enrollment request to indicate mock mode
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusTrue,
			Reason:  "AttestationMockMode",
			Message: "WARNING: Attestation verification skipped (mock mode - Keylime not enabled)",
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Get the default AttestationReference
	attestationRef, err := h.store.AttestationReference().GetDefault(ctx, orgId)
	if err != nil {
		h.log.Warnf("Failed to get default AttestationReference for attestation verification: %v", err)
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusFalse,
			Reason:  "AttestationReferenceLookupFailed",
			Message: fmt.Sprintf("Failed to get default attestation policy: %v", err),
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Check if TPM AIK is present (required)
	if attestationPkg.Data.TpmAk == nil {
		h.log.Warnf("Attestation package for %s missing required TPM AIK", attestationPkg.Metadata.EnrollmentRequestName)
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusFalse,
			Reason:  "AttestationDataIncomplete",
			Message: "Missing required TPM AIK in attestation data",
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Check if TPM Quote is present (required for v2.4 one-shot verification)
	if attestationPkg.Data.Quote == nil || *attestationPkg.Data.Quote == "" {
		h.log.Warnf("Attestation package for %s missing required TPM Quote", attestationPkg.Metadata.EnrollmentRequestName)
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusFalse,
			Reason:  "AttestationDataIncomplete",
			Message: "Missing required TPM Quote in attestation data - ensure agent has TPM enabled and collected quote",
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Check if Nonce is present (required for v2.4 one-shot verification)
	if attestationPkg.Data.Nonce == nil || *attestationPkg.Data.Nonce == "" {
		h.log.Warnf("Attestation package for %s missing required Nonce", attestationPkg.Metadata.EnrollmentRequestName)
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusFalse,
			Reason:  "AttestationDataIncomplete",
			Message: "Missing required Nonce in attestation data - ensure agent collected nonce for quote freshness",
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Use enrollment request name as device ID
	deviceID := attestationPkg.Metadata.EnrollmentRequestName

	h.log.Debugf("Attestation data for %s: has Quote=%v (%d bytes), has Nonce=%v (%d bytes), has TpmAk=%v, has TpmEk=%v",
		deviceID,
		attestationPkg.Data.Quote != nil, len(*attestationPkg.Data.Quote),
		attestationPkg.Data.Nonce != nil, len(*attestationPkg.Data.Nonce),
		attestationPkg.Data.TpmAk != nil,
		attestationPkg.Data.TpmEk != nil)

	h.log.Infof("Verifying attestation for device %s with Keylime verifier using AttestationReference %s", deviceID, *attestationRef.Metadata.Name)

	// Keylime expects the quote in format: "r<quote>:<signature>:<pcr>" (with "r" prefix indicating TPM 2.0)
	// The agent sends quote:signature:pcr
	var keylimeQuote *string
	if attestationPkg.Data.Quote != nil {
		quoteStr := *attestationPkg.Data.Quote

		// Check if quote already has "r" or "r:" prefix (for compatibility)
		if strings.HasPrefix(quoteStr, "r:") {
			quoteStr = quoteStr[2:] // Remove "r:" prefix temporarily for processing
		} else if strings.HasPrefix(quoteStr, "r") {
			quoteStr = quoteStr[1:] // Remove "r" prefix temporarily for processing
		}

		// Split quote into components: quote:signature:pcr
		parts := strings.Split(quoteStr, ":")
		var quoteParts []string
		if len(parts) != 3 {
			h.log.Warnf("Unexpected quote format, expected 3 parts (quote:signature:pcr), got %d parts", len(parts))
			formattedQuote := "r" + *attestationPkg.Data.Quote
			keylimeQuote = &formattedQuote
			// Can't parse PCRs from malformed quote
			quoteParts = nil
		} else {
			quoteParts = parts

			// Decode each part to check actual binary sizes
			quoteBytes, err1 := base64.StdEncoding.DecodeString(parts[0])
			sigBytes, err2 := base64.StdEncoding.DecodeString(parts[1])
			pcrBytes, err3 := base64.StdEncoding.DecodeString(parts[2])

			if err1 == nil && err2 == nil && err3 == nil {
				h.log.Infof("Quote component sizes (decoded): quote=%d bytes, sig=%d bytes, pcr=%d bytes",
					len(quoteBytes), len(sigBytes), len(pcrBytes))
				// Log first few bytes of each to debug format
				if len(quoteBytes) >= 4 {
					h.log.Infof("Quote first 4 bytes (hex): %02x %02x %02x %02x (should be ff 54 43 47 = TPM_GENERATED_VALUE)",
						quoteBytes[0], quoteBytes[1], quoteBytes[2], quoteBytes[3])
				}
				if len(sigBytes) >= 4 {
					h.log.Infof("Signature first 4 bytes (hex): %02x %02x %02x %02x",
						sigBytes[0], sigBytes[1], sigBytes[2], sigBytes[3])
				}
			} else {
				h.log.Warnf("Failed to decode quote components: quote_err=%v, sig_err=%v, pcr_err=%v", err1, err2, err3)
			}

			// Agent already sends quote/signature/pcr in correct format
			// Quote is TPMS_ATTEST (starts with magic 0xFF544347)
			// Signature is TPMT_SIGNATURE
			// No TPM2B wrappers to strip - just prepend "r" prefix
			formattedQuote := "r" + parts[0] + ":" + parts[1] + ":" + parts[2]
			keylimeQuote = &formattedQuote
			h.log.Infof("Sending quote to Keylime (no stripping needed): total %d chars", len(formattedQuote))
		}

		// Parse PCRs from the quote to dynamically construct TPM policy mask
		if quoteParts != nil {
			pcrs, err := parsePCRSelectionFromQuote(quoteParts)
			if err != nil {
				h.log.Warnf("Failed to parse PCR selection from quote: %v", err)
			} else {
				h.log.Infof("Detected %d PCRs in quote: %v", len(pcrs), pcrs)

				// Build TPM policy with mask matching the PCRs in the quote
				dynamicPolicy, err := buildTPMPolicyFromPCRs(pcrs, attestationRef.Spec.TpmPolicy)
				if err != nil {
					h.log.Warnf("Failed to build dynamic TPM policy: %v", err)
				} else {
					h.log.Infof("Using dynamic TPM policy based on quote PCRs: %s", dynamicPolicy)
					// Override the TPM policy with dynamically constructed one
					attestationRef.Spec.TpmPolicy = &dynamicPolicy
				}
			}
		}
	}

	// Call the Keylime verifier for one-shot verification (v2.5 API)
	// Note: Keylime's _tpm2_checkquote() expects base64-encoded TPM2B_PUBLIC,
	// which it internally converts to PEM before passing to the low-level checkquote() function.
	// The agent already sends TPM2B_PUBLIC in base64 format, so we pass it directly.
	resultStatus, err := h.keylimeClient.VerifyAttestation(
		ctx,
		deviceID,
		*attestationPkg.Data.TpmAk,
		attestationPkg.Data.TpmEk,
		keylimeQuote,
		attestationPkg.Data.Nonce,
		attestationRef.Spec.MbPolicy,
		attestationRef.Spec.RuntimePolicy,
		attestationRef.Spec.TpmPolicy,
		attestationPkg.Data.ImaMeasurementList,
		attestationPkg.Data.MbLog,
	)
	if err != nil {
		h.log.Errorf("Keylime attestation verification failed for %s: %v", deviceID, err)
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusFalse,
			Reason:  "KeylimeVerificationFailed",
			Message: fmt.Sprintf("Keylime attestation verification failed: %v", err),
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Check the response
	if resultStatus != "Success" {
		h.log.Warnf("Keylime verifier returned non-success status for %s: %s", deviceID, resultStatus)
		condition := domain.Condition{
			Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
			Status:  domain.ConditionStatusFalse,
			Reason:  "KeylimeVerificationRejected",
			Message: fmt.Sprintf("Keylime verifier rejected attestation: %s", resultStatus),
		}
		domain.SetStatusCondition(&er.Status.Conditions, condition)
		return nil
	}

	// Successful verification
	h.log.Infof("Keylime verifier successfully verified attestation for device %s", deviceID)
	condition := domain.Condition{
		Type:    domain.ConditionTypeEnrollmentRequestAttestationVerified,
		Status:  domain.ConditionStatusTrue,
		Reason:  "KeylimeVerificationSucceeded",
		Message: fmt.Sprintf("Attestation verified by Keylime verifier using policy %s", *attestationRef.Metadata.Name),
	}
	domain.SetStatusCondition(&er.Status.Conditions, condition)

	return nil
}

func approveAndSignEnrollmentRequest(ctx context.Context, ca *crypto.CAClient, enrollmentRequest *domain.EnrollmentRequest, approval *domain.EnrollmentRequestApprovalStatus) error {
	if enrollmentRequest == nil {
		return errors.New("approveAndSignEnrollmentRequest: enrollmentRequest is nil")
	}

	request, _, err := newSignRequestFromEnrollment(ca.Cfg, enrollmentRequest)
	if err != nil {
		return fmt.Errorf("approveAndSignEnrollmentRequest: %w", err)
	}

	certData, err := signer.SignAsPEM(ctx, ca, request)
	if err != nil {
		return fmt.Errorf("approveAndSignEnrollmentRequest: %w", err)
	}

	// preserve existing conditions when approving
	existingConditions := []domain.Condition{}
	if enrollmentRequest.Status != nil && enrollmentRequest.Status.Conditions != nil {
		existingConditions = enrollmentRequest.Status.Conditions
	}

	enrollmentRequest.Status = &domain.EnrollmentRequestStatus{
		Certificate: lo.ToPtr(string(certData)),
		Conditions:  existingConditions,
		Approval:    approval,
	}

	// union user-provided labels with agent-provided labels
	if enrollmentRequest.Spec.Labels != nil {
		for k, v := range *enrollmentRequest.Spec.Labels {
			// don't override user-provided labels
			if _, ok := (*enrollmentRequest.Status.Approval.Labels)[k]; !ok {
				(*enrollmentRequest.Status.Approval.Labels)[k] = v
			}
		}
	}

	condition := domain.Condition{
		Type:    domain.ConditionTypeEnrollmentRequestApproved,
		Status:  domain.ConditionStatusTrue,
		Reason:  "ManuallyApproved",
		Message: "Approved by " + approval.ApprovedBy,
	}
	domain.SetStatusCondition(&enrollmentRequest.Status.Conditions, condition)
	return nil
}

func addStatusIfNeeded(enrollmentRequest *domain.EnrollmentRequest) {
	if enrollmentRequest.Status == nil {
		enrollmentRequest.Status = &domain.EnrollmentRequestStatus{
			Certificate: nil,
			Conditions:  []domain.Condition{},
		}
	}
}

func (h *ServiceHandler) createDeviceFromEnrollmentRequest(ctx context.Context, orgId uuid.UUID, enrollmentRequest *domain.EnrollmentRequest) error {
	deviceStatus := domain.NewDeviceStatus()
	deviceStatus.Lifecycle = domain.DeviceLifecycleStatus{Status: "Enrolled"}

	// Check if TPM was verified during enrollment request creation
	isTPMVerified := false
	var tpmVerificationError string
	if enrollmentRequest.Status != nil {
		if condition := domain.FindStatusCondition(enrollmentRequest.Status.Conditions, domain.ConditionTypeEnrollmentRequestTPMVerified); condition != nil {
			isTPMVerified = condition.Status == domain.ConditionStatusTrue
			if !isTPMVerified {
				tpmVerificationError = condition.Message
			}
		}
	}

	// always set device integrity status based on TPM verification result
	now := time.Now()
	if isTPMVerified {
		deviceStatus.Integrity = domain.DeviceIntegrityStatus{
			Status:       domain.DeviceIntegrityStatusVerified,
			Info:         lo.ToPtr("All integrity checks completed successfully"),
			LastVerified: &now,
			DeviceIdentity: &domain.DeviceIntegrityCheckStatus{
				Status: domain.DeviceIntegrityCheckStatusVerified,
				Info:   lo.ToPtr("Device identity verified through certificate chain"),
			},
			Tpm: &domain.DeviceIntegrityCheckStatus{
				Status: domain.DeviceIntegrityCheckStatusVerified,
				Info:   lo.ToPtr("TPM chain of trust verified"),
			},
		}
	} else {
		// Check if this was a TCG CSR enrollment attempt that failed verification
		csrBytes := []byte(enrollmentRequest.Spec.Csr)

		if !tpm.IsTCGCSRFormat(csrBytes) {
			decodedBytes, err := base64.StdEncoding.DecodeString(enrollmentRequest.Spec.Csr)
			if err == nil && tpm.IsTCGCSRFormat(decodedBytes) {
				csrBytes = decodedBytes
			}
		}

		if tpm.IsTCGCSRFormat(csrBytes) {
			tpmErrorMsg := "TPM attestation verification failed"
			deviceIdentityMsg := "Device identity verification failed"

			if tpmVerificationError != "" {
				if strings.Contains(tpmVerificationError, "TPM CA certificates not configured") {
					tpmErrorMsg = "TPM CA certificates not configured - cannot verify chain"
					deviceIdentityMsg = "Cannot verify identity - TPM CA certificates not configured"
				} else {
					tpmErrorMsg = fmt.Sprintf("TPM verification failed: %s", tpmVerificationError)
					deviceIdentityMsg = fmt.Sprintf("Identity verification failed: %s", tpmVerificationError)
				}
			}

			deviceStatus.Integrity = domain.DeviceIntegrityStatus{
				Status:       domain.DeviceIntegrityStatusFailed,
				Info:         lo.ToPtr("Integrity verification failed"),
				LastVerified: &now,
				DeviceIdentity: &domain.DeviceIntegrityCheckStatus{
					Status: domain.DeviceIntegrityCheckStatusFailed,
					Info:   lo.ToPtr(deviceIdentityMsg),
				},
				Tpm: &domain.DeviceIntegrityCheckStatus{
					Status: domain.DeviceIntegrityCheckStatusFailed,
					Info:   lo.ToPtr(tpmErrorMsg),
				},
			}
		} else {
			// Device enrolled without TPM - integrity verification not supported
			deviceStatus.Integrity = domain.DeviceIntegrityStatus{
				Status: domain.DeviceIntegrityStatusUnsupported,
				Info:   lo.ToPtr("TPM not present or not enabled on this device"),
				DeviceIdentity: &domain.DeviceIntegrityCheckStatus{
					Status: domain.DeviceIntegrityCheckStatusUnsupported,
				},
				Tpm: &domain.DeviceIntegrityCheckStatus{
					Status: domain.DeviceIntegrityCheckStatusUnsupported,
				},
			}
		}
	}

	name := lo.FromPtr(enrollmentRequest.Metadata.Name)
	apiResource := &domain.Device{
		Metadata: domain.ObjectMeta{
			Name: &name,
		},
		Status: &deviceStatus,
	}
	if errs := apiResource.Validate(); len(errs) > 0 {
		return fmt.Errorf("failed validating new device: %w", errors.Join(errs...))
	}
	if enrollmentRequest.Status != nil && enrollmentRequest.Status.Approval != nil {
		apiResource.Metadata.Labels = enrollmentRequest.Status.Approval.Labels
	}

	// Transfer awaitingReconnect annotation from enrollment request to device if present
	if enrollmentRequest.Metadata.Annotations != nil {
		if awaitingReconnect, exists := (*enrollmentRequest.Metadata.Annotations)[domain.DeviceAnnotationAwaitingReconnect]; exists && awaitingReconnect == "true" {
			if apiResource.Metadata.Annotations == nil {
				apiResource.Metadata.Annotations = &map[string]string{}
			}
			(*apiResource.Metadata.Annotations)[domain.DeviceAnnotationAwaitingReconnect] = "true"

			// Set device status to awaiting reconnection
			deviceStatus.Summary = domain.DeviceSummaryStatus{
				Status: domain.DeviceSummaryStatusAwaitingReconnect,
				Info:   lo.ToPtr(common.DeviceStatusInfoAwaitingReconnect),
			}
			// Add awaiting reconnection key to KV store
			key := kvstore.AwaitingReconnectionKey{
				OrgID:      orgId,
				DeviceName: name,
			}
			keyStr := key.ComposeKey()
			_, err := h.kvStore.SetNX(ctx, keyStr, []byte("true"))
			if err != nil {
				h.log.WithError(err).Errorf("Failed to add awaiting reconnection key for device %s in org %s", name, orgId)
			}
		}
	}
	_ = common.UpdateServiceSideStatus(ctx, orgId, apiResource, h.store, h.log)

	_, _, err := h.store.Device().CreateOrUpdate(ctx, orgId, apiResource, nil, false, func(ctx context.Context, before *domain.Device, after *domain.Device) error {
		// Prevent overwriting existing devices during enrollment request approval
		if before != nil {
			return fmt.Errorf("device %s already exists and cannot be overwritten during enrollment request approval", *after.Metadata.Name)
		}
		return nil
	}, func(ctx context.Context, resourceKind domain.ResourceKind, orgId uuid.UUID, name string, oldResource, newResource interface{}, created bool, err error) {
		// Only invoke callback on success
		if err == nil {
			h.callbackDeviceUpdated(ctx, resourceKind, orgId, name, oldResource, newResource, created, err)
		}
	})
	return err
}

func (h *ServiceHandler) CreateEnrollmentRequest(ctx context.Context, orgId uuid.UUID, er domain.EnrollmentRequest) (*domain.EnrollmentRequest, domain.Status) {
	er.Status = nil
	addStatusIfNeeded(&er)

	// don't set fields that are managed by the service for external requests
	if !IsInternalRequest(ctx) {
		NilOutManagedObjectMetaProperties(&er.Metadata)
	}

	// Check if knownRenderedVersion is provided and not "0", add awaitingReconnect annotation
	if er.Spec.KnownRenderedVersion != nil && *er.Spec.KnownRenderedVersion != "" && *er.Spec.KnownRenderedVersion != "0" {
		annotations := util.EnsureMap(lo.FromPtr(er.Metadata.Annotations))
		annotations[domain.DeviceAnnotationAwaitingReconnect] = "true"
		er.Metadata.Annotations = &annotations
		h.log.Infof("Adding awaitingReconnect annotation for knownRenderedVersion: %s", *er.Spec.KnownRenderedVersion)
	}

	if errs := er.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}

	// Extract attestation data if present and process with Keylime verifier
	attestationPkg := extractAttestationData(&er, h.log)
	if attestationPkg != nil {
		if err := h.processAttestationWithKeylime(ctx, orgId, attestationPkg, &er); err != nil {
			h.log.Errorf("Failed to process attestation with Keylime: %v", err)
		}
	}

	request, isTPM, err := newSignRequestFromEnrollment(h.ca.Cfg, &er)
	if err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if err := signer.Verify(ctx, h.ca, request); err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if isTPM {
		if err := h.verifyTPMEnrollmentRequest(&er, *er.Metadata.Name); err != nil {
			return nil, domain.StatusBadRequest(err.Error())
		}
	}
	if _, isAgent := ctx.Value(consts.AgentCtxKey).(string); isAgent {
		if h.agentGate.Acquire(ctx, 1) == nil {
			defer h.agentGate.Release(1)
		}
	}

	// Use fromAPI=false for internal requests to preserve annotations
	result, err := h.store.EnrollmentRequest().CreateWithFromAPI(ctx, orgId, &er, false, h.callbackEnrollmentRequestUpdated)
	return result, StoreErrorToApiStatus(err, true, domain.EnrollmentRequestKind, er.Metadata.Name)
}

func (h *ServiceHandler) ListEnrollmentRequests(ctx context.Context, orgId uuid.UUID, params domain.ListEnrollmentRequestsParams) (*domain.EnrollmentRequestList, domain.Status) {
	listParams, status := prepareListParams(params.Continue, params.LabelSelector, params.FieldSelector, params.Limit)
	if status != domain.StatusOK() {
		return nil, status
	}

	result, err := h.store.EnrollmentRequest().List(ctx, orgId, *listParams)
	if err == nil {
		return result, domain.StatusOK()
	}

	var se *selector.SelectorError

	switch {
	case selector.AsSelectorError(err, &se):
		return nil, domain.StatusBadRequest(se.Error())
	default:
		return nil, domain.StatusInternalServerError(err.Error())
	}
}

func (h *ServiceHandler) GetEnrollmentRequest(ctx context.Context, orgId uuid.UUID, name string) (*domain.EnrollmentRequest, domain.Status) {
	if _, isAgent := ctx.Value(consts.AgentCtxKey).(string); isAgent {
		if h.agentGate.Acquire(ctx, 1) == nil {
			defer h.agentGate.Release(1)
		}
	}
	result, err := h.store.EnrollmentRequest().Get(ctx, orgId, name)
	return result, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
}

func (h *ServiceHandler) ReplaceEnrollmentRequest(ctx context.Context, orgId uuid.UUID, name string, er domain.EnrollmentRequest) (*domain.EnrollmentRequest, domain.Status) {
	// don't set fields that are managed by the service
	er.Status = nil
	addStatusIfNeeded(&er)
	NilOutManagedObjectMetaProperties(&er.Metadata)

	if errs := er.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}
	err := h.allowCreationOrUpdate(ctx, orgId, name)
	if err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if name != *er.Metadata.Name {
		return nil, domain.StatusBadRequest("resource name specified in metadata does not match name in path")
	}

	// Extract attestation data if present and process with Keylime verifier
	attestationPkg := extractAttestationData(&er, h.log)
	if attestationPkg != nil {
		if err := h.processAttestationWithKeylime(ctx, orgId, attestationPkg, &er); err != nil {
			h.log.Errorf("Failed to process attestation with Keylime: %v", err)
		}
	}

	request, isTPM, err := newSignRequestFromEnrollment(h.ca.Cfg, &er)
	if err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if err := signer.Verify(ctx, h.ca, request); err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if isTPM {
		if err := h.verifyTPMEnrollmentRequest(&er, name); err != nil {
			return nil, domain.StatusBadRequest(err.Error())
		}
	}

	result, created, err := h.store.EnrollmentRequest().CreateOrUpdate(ctx, orgId, &er, h.callbackEnrollmentRequestUpdated)
	return result, StoreErrorToApiStatus(err, created, domain.EnrollmentRequestKind, &name)
}

// Only metadata.labels and spec can be patched. If we try to patch other fields, HTTP 400 Bad Request is returned.
func (h *ServiceHandler) PatchEnrollmentRequest(ctx context.Context, orgId uuid.UUID, name string, patch domain.PatchRequest) (*domain.EnrollmentRequest, domain.Status) {
	currentObj, err := h.store.EnrollmentRequest().Get(ctx, orgId, name)
	if err != nil {
		return nil, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
	}

	newObj := &domain.EnrollmentRequest{}
	err = ApplyJSONPatch(ctx, currentObj, newObj, patch, "/enrollmentrequests/"+name)
	if err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}

	if errs := newObj.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}
	if errs := currentObj.ValidateUpdate(newObj); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}

	NilOutManagedObjectMetaProperties(&newObj.Metadata)
	newObj.Metadata.ResourceVersion = nil

	request, isTPM, err := newSignRequestFromEnrollment(h.ca.Cfg, newObj)
	if err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if err := signer.Verify(ctx, h.ca, request); err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}
	if isTPM {
		if err := h.verifyTPMEnrollmentRequest(newObj, name); err != nil {
			return nil, domain.StatusBadRequest(err.Error())
		}
	}

	result, err := h.store.EnrollmentRequest().Update(ctx, orgId, newObj, h.callbackEnrollmentRequestUpdated)
	return result, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
}

func (h *ServiceHandler) DeleteEnrollmentRequest(ctx context.Context, orgId uuid.UUID, name string) domain.Status {
	exists, err := h.deviceExists(ctx, orgId, name)
	if err != nil {
		return StoreErrorToApiStatus(err, false, domain.DeviceKind, &name)
	}

	if exists {
		return domain.StatusConflict(fmt.Sprintf("cannot delete ER %q: device exists", name))
	}

	err = h.store.EnrollmentRequest().Delete(ctx, orgId, name, h.callbackEnrollmentRequestDeleted)
	return StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
}

func (h *ServiceHandler) GetEnrollmentRequestStatus(ctx context.Context, orgId uuid.UUID, name string) (*domain.EnrollmentRequest, domain.Status) {
	result, err := h.store.EnrollmentRequest().Get(ctx, orgId, name)
	return result, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
}

func (h *ServiceHandler) ApproveEnrollmentRequest(ctx context.Context, orgId uuid.UUID, name string, approval domain.EnrollmentRequestApproval) (*domain.EnrollmentRequestApprovalStatus, domain.Status) {
	if errs := approval.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}
	enrollmentReq, err := h.store.EnrollmentRequest().Get(ctx, orgId, name)
	if err != nil {
		return nil, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
	}

	approvalStatusToReturn := enrollmentReq.Status.Approval

	// if the enrollment request was already approved we should not try to approve it one more time
	if approval.Approved {
		if domain.IsStatusConditionTrue(enrollmentReq.Status.Conditions, domain.ConditionTypeEnrollmentRequestApproved) {
			return nil, domain.StatusBadRequest("Enrollment request is already approved")
		}

		identity, ok := contextutil.GetMappedIdentityFromContext(ctx)
		if !ok {
			status := domain.StatusInternalServerError("failed to retrieve user identity while approving enrollment request")
			h.CreateEvent(ctx, orgId, common.GetEnrollmentRequestApprovalFailedEvent(ctx, name, status, h.log))
			return nil, status
		}

		approvedBy := "unknown"
		if identity != nil && len(identity.GetUsername()) > 0 {
			approvedBy = identity.GetUsername()
		}

		approvalStatus := domain.EnrollmentRequestApprovalStatus{
			Approved:   approval.Approved,
			Labels:     approval.Labels,
			ApprovedAt: time.Now(),
			ApprovedBy: approvedBy,
		}
		approvalStatusToReturn = &approvalStatus

		err = approveAndSignEnrollmentRequest(ctx, h.ca, enrollmentReq, &approvalStatus)
		if err != nil {
			status := domain.StatusBadRequest(fmt.Sprintf("Error approving and signing enrollment request: %v", err.Error()))
			h.CreateEvent(ctx, orgId, common.GetEnrollmentRequestApprovalFailedEvent(ctx, name, status, h.log))
			return nil, status
		}

		// in case of error we return 500 as it will be caused by creating device in db and not by problem with enrollment request
		if err := h.createDeviceFromEnrollmentRequest(ctx, orgId, enrollmentReq); err != nil {
			status := domain.StatusInternalServerError(fmt.Sprintf("error creating device from enrollment request: %v", err))
			h.CreateEvent(ctx, orgId, common.GetEnrollmentRequestApprovalFailedEvent(ctx, name, status, h.log))
			return nil, status
		}
	}

	// Update the enrollment request status using the specific approval callback
	_, err = h.store.EnrollmentRequest().UpdateStatus(ctx, orgId, enrollmentReq, h.callbackEnrollmentRequestApproved)
	return approvalStatusToReturn, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
}

func (h *ServiceHandler) ReplaceEnrollmentRequestStatus(ctx context.Context, orgId uuid.UUID, name string, er domain.EnrollmentRequest) (*domain.EnrollmentRequest, domain.Status) {
	addStatusIfNeeded(&er)

	result, err := h.store.EnrollmentRequest().UpdateStatus(ctx, orgId, &er, h.callbackEnrollmentRequestUpdated)
	return result, StoreErrorToApiStatus(err, false, domain.EnrollmentRequestKind, &name)
}

func newSignRequestFromEnrollment(cfg *ca.Config, er *domain.EnrollmentRequest) (signer.SignRequest, bool, error) {
	csrData, isTPM, err := tpm.NormalizeEnrollmentCSR(er.Spec.Csr)
	if err != nil {
		return nil, false, fmt.Errorf("failed to normalize CSR: %w", err)
	}

	var opts []signer.SignRequestOption
	if er.Status != nil && er.Status.Certificate != nil {
		certBytes := []byte(*er.Status.Certificate)
		opts = append(opts, signer.WithIssuedCertificateBytes(certBytes))
	}

	if er.Metadata.Name != nil {
		opts = append(opts, signer.WithResourceName(*er.Metadata.Name))
	}

	request, err := signer.NewSignRequestFromBytes(cfg.DeviceManagementSignerName, csrData, opts...)

	if err != nil {
		return nil, isTPM, err
	}

	return request, isTPM, nil
}

func (h *ServiceHandler) allowCreationOrUpdate(ctx context.Context, orgId uuid.UUID, name string) error {
	device, err := h.store.Device().Get(ctx, orgId, name)
	if errors.Is(err, flterrors.ErrResourceNotFound) {
		return nil // Device not found: allow to create or update
	}
	if device != nil {
		return flterrors.ErrDuplicateName // Duplicate name: creation blocked
	}
	return err
}

// deviceExists checks if a device with the given name exists in the store.
// Error is returned if there is an error other than ErrResourceNotFound.
func (h *ServiceHandler) deviceExists(ctx context.Context, orgId uuid.UUID, name string) (bool, error) {
	dev, err := h.store.Device().Get(ctx, orgId, name)
	if errors.Is(err, flterrors.ErrResourceNotFound) {
		return false, nil
	}
	return dev != nil, err
}

// callbackEnrollmentRequestUpdated is the enrollment request-specific callback that handles enrollment request events
func (h *ServiceHandler) callbackEnrollmentRequestUpdated(ctx context.Context, resourceKind domain.ResourceKind, orgId uuid.UUID, name string, oldResource, newResource interface{}, created bool, err error) {
	h.eventHandler.HandleEnrollmentRequestUpdatedEvents(ctx, resourceKind, orgId, name, oldResource, newResource, created, err)
}

// callbackEnrollmentRequestDeleted is the enrollment request-specific callback that handles enrollment request deletion events
func (h *ServiceHandler) callbackEnrollmentRequestDeleted(ctx context.Context, resourceKind domain.ResourceKind, orgId uuid.UUID, name string, oldResource, newResource interface{}, created bool, err error) {
	h.eventHandler.HandleGenericResourceDeletedEvents(ctx, resourceKind, orgId, name, oldResource, newResource, created, err)
}

// callbackEnrollmentRequestApproved is the enrollment request-specific callback that handles enrollment request approval events
func (h *ServiceHandler) callbackEnrollmentRequestApproved(ctx context.Context, resourceKind domain.ResourceKind, orgId uuid.UUID, name string, oldResource, newResource interface{}, created bool, err error) {
	h.eventHandler.HandleEnrollmentRequestApprovedEvents(ctx, resourceKind, orgId, name, oldResource, newResource, created, err)
}
