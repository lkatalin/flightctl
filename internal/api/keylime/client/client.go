package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	keylimeapi "github.com/flightctl/flightctl/api/keylime/v1beta1"
	"github.com/sirupsen/logrus"
)

// Client is a client for the Keylime verifier service
type Client struct {
	baseURL    string
	httpClient *http.Client
	log        logrus.FieldLogger
}

// NewClient creates a new Keylime verifier client
func NewClient(baseURL string, log logrus.FieldLogger) *Client {
	// Create HTTP client with TLS configuration
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 - required for demo with self-signed certs
	}

	// Load client certificates if available (for mTLS with Keylime)
	// Certificates are mounted from Kubernetes Secret if available
	clientCertPath := "/etc/flightctl/keylime/client.crt"
	clientKeyPath := "/etc/flightctl/keylime/client.key"
	caCertPath := "/etc/flightctl/keylime/ca.crt"

	if fileExists(clientCertPath) && fileExists(clientKeyPath) {
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			log.Warnf("Failed to load Keylime client certificates from %s and %s: %v", clientCertPath, clientKeyPath, err)
		} else {
			tlsConfig.Certificates = []tls.Certificate{cert}
			log.Infof("Loaded Keylime client certificates for mTLS")

			// Load CA certificate if available
			if fileExists(caCertPath) {
				caCert, err := os.ReadFile(caCertPath)
				if err != nil {
					log.Warnf("Failed to read Keylime CA certificate from %s: %v", caCertPath, err)
				} else {
					caCertPool := x509.NewCertPool()
					if caCertPool.AppendCertsFromPEM(caCert) {
						tlsConfig.RootCAs = caCertPool
						// Keylime's auto-generated certificates use "server" as the hostname
						// Override ServerName to match the certificate instead of the DNS name
						tlsConfig.ServerName = "server"
						tlsConfig.InsecureSkipVerify = false // Use proper verification with CA
						log.Infof("Loaded Keylime CA certificate, TLS verification enabled with ServerName=server")
					}
				}
			}
		}
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tr,
		},
		log: log,
	}
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// logAttestationToFile logs the attestation request and response to a dedicated file
func logAttestationToFile(deviceID string, requestData map[string]interface{}, responseData map[string]interface{}, requestTime, responseTime time.Time, err error) {
	logFile := "/tmp/attestation_log.txt"
	f, fileErr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr != nil {
		// Silently fail if we can't open log file
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "\n================================================================================\n")
	fmt.Fprintf(f, "ATTESTATION REQUEST/RESPONSE LOG\n")
	fmt.Fprintf(f, "================================================================================\n")
	fmt.Fprintf(f, "Enrollment Request ID: %s\n", deviceID)
	fmt.Fprintf(f, "Request Sent:          %s\n", requestTime.Format(time.RFC3339Nano))
	fmt.Fprintf(f, "Response Received:     %s\n", responseTime.Format(time.RFC3339Nano))
	fmt.Fprintf(f, "Duration:              %v\n", responseTime.Sub(requestTime))
	fmt.Fprintf(f, "\n")

	fmt.Fprintf(f, "--- REQUEST SENT TO KEYLIME ---\n")
	requestJSON, _ := json.MarshalIndent(requestData, "", "  ")

	// Truncate very large fields for readability
	var requestPretty map[string]interface{}
	json.Unmarshal(requestJSON, &requestPretty)
	if data, ok := requestPretty["data"].(map[string]interface{}); ok {
		if rp, exists := data["runtime_policy"]; exists {
			if rpStr, ok := rp.(string); ok && len(rpStr) > 500 {
				data["runtime_policy"] = fmt.Sprintf("<TRUNCATED: %d bytes total, first 200 chars: %.200s...>", len(rpStr), rpStr)
			}
		}
		if quote, exists := data["quote"]; exists {
			if quoteStr, ok := quote.(string); ok && len(quoteStr) > 500 {
				data["quote"] = fmt.Sprintf("<TRUNCATED: %d bytes total>", len(quoteStr))
			}
		}
		if ima, exists := data["ima_measurement_list"]; exists {
			if imaStr, ok := ima.(string); ok && len(imaStr) > 500 {
				data["ima_measurement_list"] = fmt.Sprintf("<TRUNCATED: %d bytes total>", len(imaStr))
			}
		}
	}
	requestPrettyJSON, _ := json.MarshalIndent(requestPretty, "", "  ")
	fmt.Fprintf(f, "%s\n", string(requestPrettyJSON))

	fmt.Fprintf(f, "\n--- RESPONSE RECEIVED FROM KEYLIME ---\n")
	if err != nil {
		fmt.Fprintf(f, "ERROR: %v\n", err)
	} else {
		responseJSON, _ := json.MarshalIndent(responseData, "", "  ")
		fmt.Fprintf(f, "%s\n", string(responseJSON))
	}

	fmt.Fprintf(f, "\n")
}

// VerifyAttestation sends TPM attestation data to the Keylime verifier for one-shot verification.
// Uses the /v2.5/verify/evidence endpoint which does NOT require agent registration or polling.
// This is a blocking call that returns immediate verification results.
// Returns the verification result status string ("Success" or error details) and any error.
func (c *Client) VerifyAttestation(ctx context.Context, deviceID string, aikTpm string, ekTpm *string, quote, nonce *string, mbPolicy, runtimePolicy, tpmPolicy *string, imaMeasurementList, mbLog *string) (string, error) {
	requestTime := time.Now()
	url := fmt.Sprintf("%s/v2.5/verify/evidence", c.baseURL)

	// v2.5 API requires quote and nonce for one-shot verification
	if quote == nil || *quote == "" {
		return "", fmt.Errorf("TPM quote is required for verification")
	}
	if nonce == nil || *nonce == "" {
		return "", fmt.Errorf("nonce is required for verification")
	}

	// Debug: Log quote format for debugging "Invalid quote type A" error
	quotePrefix := *quote
	if len(quotePrefix) > 20 {
		quotePrefix = quotePrefix[:20]
	}
	c.log.Infof("Quote data prefix (first 20 chars): %s", quotePrefix)
	c.log.Infof("Quote length: %d bytes", len(*quote))

	// Build request with nested data structure (Keylime master branch format)
	// The latest Keylime expects all attestation parameters nested under a "data" object
	req := keylimeapi.VerifyEvidenceRequest{
		Type: "tpm",
	}

	// Populate the nested Data object
	req.Data.Quote = *quote
	req.Data.Nonce = *nonce
	req.Data.HashAlg = "sha256" // Default hash algorithm
	req.Data.TpmAk = aikTpm

	if ekTpm != nil {
		req.Data.TpmEk = ekTpm
	}
	if tpmPolicy != nil && *tpmPolicy != "" {
		req.Data.TpmPolicy = tpmPolicy
	} else {
		// Default TPM policy with mask 0x0 (no PCR verification for demo)
		defaultPolicy := `{"mask":"0x0"}`
		req.Data.TpmPolicy = &defaultPolicy
	}
	if runtimePolicy != nil && *runtimePolicy != "" {
		c.log.Infof("DEBUG: Setting req.Data.RuntimePolicy, size: %d bytes", len(*runtimePolicy))
		c.log.Infof("DEBUG: RuntimePolicy first 100 chars: %.100s", *runtimePolicy)

		// Parse and log excludes to verify they're being sent
		var policyData map[string]interface{}
		if err := json.Unmarshal([]byte(*runtimePolicy), &policyData); err == nil {
			if excludes, ok := policyData["excludes"]; ok {
				excludesJSON, _ := json.Marshal(excludes)
				c.log.Infof("DEBUG: RuntimePolicy excludes: %s", string(excludesJSON))
			} else {
				c.log.Warnf("DEBUG: RuntimePolicy has NO excludes field")
			}
		} else {
			c.log.Warnf("DEBUG: Failed to parse RuntimePolicy JSON: %v", err)
		}

		req.Data.RuntimePolicy = runtimePolicy
	} else {
		c.log.Warnf("DEBUG: RuntimePolicy is nil or empty, NOT setting req.Data.RuntimePolicy")
	}
	// Only send mbPolicy and mbLog if we have a non-empty measured boot policy
	// Check if mbPolicy is effectively empty (nil, empty string, or empty JSON object)
	mbPolicyIsEmpty := mbPolicy == nil || *mbPolicy == ""
	if !mbPolicyIsEmpty {
		// Trim whitespace and check if it's an empty JSON object
		trimmed := strings.TrimSpace(*mbPolicy)
		if trimmed == "{}" || trimmed == "" {
			mbPolicyIsEmpty = true
		}
	}

	if !mbPolicyIsEmpty {
		req.Data.MbPolicy = mbPolicy
		// Only send measured boot log if we have a measured boot policy
		// Otherwise Keylime will parse it and fail on warnings without policy guidance
		if mbLog != nil {
			req.Data.MbLog = mbLog
		}
	}
	if imaMeasurementList != nil {
		req.Data.ImaMeasurementList = imaMeasurementList
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug: Log the request being sent to Keylime
	c.log.Infof("Sending request to Keylime v2.5 API at %s", url)
	c.log.Infof("Request body size: %d bytes", len(body))

	// Debug: Check what fields are in the marshaled JSON and store for logging
	var checkReq map[string]interface{}
	json.Unmarshal(body, &checkReq)

	if data, ok := checkReq["data"].(map[string]interface{}); ok {
		c.log.Infof("DEBUG: Marshaled request contains 'data' object with %d fields", len(data))
		if rp, exists := data["runtime_policy"]; exists {
			rpStr := fmt.Sprintf("%v", rp)
			c.log.Infof("DEBUG: runtime_policy field EXISTS in marshaled JSON, size: %d bytes", len(rpStr))
			c.log.Infof("DEBUG: runtime_policy first 200 chars: %.200s", rpStr)
		} else {
			c.log.Warnf("DEBUG: runtime_policy field NOT FOUND in marshaled JSON data object")
		}
		if tp, exists := data["tpm_policy"]; exists {
			c.log.Infof("DEBUG: tpm_policy field exists: %v", tp)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	c.log.Debugf("Verifying attestation for device %s with Keylime verifier at %s", deviceID, url)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		logAttestationToFile(deviceID, checkReq, nil, requestTime, time.Now(), fmt.Errorf("failed to send request: %w", err))
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logAttestationToFile(deviceID, checkReq, nil, requestTime, time.Now(), fmt.Errorf("failed to read response: %w", err))
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]interface{}
		json.Unmarshal(respBody, &errorResp)
		logAttestationToFile(deviceID, checkReq, errorResp, requestTime, time.Now(), fmt.Errorf("keylime verifier returned status %d: %s", resp.StatusCode, string(respBody)))
		return "", fmt.Errorf("keylime verifier returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Debug: Log raw response from Keylime
	c.log.Infof("Raw Keylime response: %s", string(respBody))

	responseTime := time.Now()

	var response keylimeapi.VerifyEvidenceResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		// Log failed parse
		logAttestationToFile(deviceID, checkReq, nil, requestTime, responseTime, fmt.Errorf("failed to unmarshal response: %w", err))
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to map for logging
	var responseMap map[string]interface{}
	json.Unmarshal(respBody, &responseMap)

	// Valid is a bool pointer: true = valid, false = invalid
	validBool := response.Results.Valid != nil && *response.Results.Valid
	c.log.Infof("Keylime verifier completed one-shot attestation for device %s: valid=%v", deviceID, validBool)
	if response.Results.Failures != nil {
		c.log.Infof("Failures array length: %d", len(*response.Results.Failures))
		c.log.Infof("Failures content: %+v", *response.Results.Failures)
	} else {
		c.log.Infof("Failures is nil")
	}

	// Log the complete attestation exchange to file
	var logErr error
	if !validBool {
		if response.Results.Failures != nil && len(*response.Results.Failures) > 0 {
			failuresJSON, _ := json.Marshal(*response.Results.Failures)
			logErr = fmt.Errorf("attestation validation failed: %s", string(failuresJSON))
		} else {
			logErr = fmt.Errorf("attestation validation failed (no failure details provided)")
		}
	}
	logAttestationToFile(deviceID, checkReq, responseMap, requestTime, responseTime, logErr)

	// Check if validation succeeded
	if validBool {
		return "Success", nil
	}

	// Validation failed - format failure details
	if response.Results.Failures != nil && len(*response.Results.Failures) > 0 {
		failuresJSON, err := json.Marshal(*response.Results.Failures)
		if err != nil {
			return "", fmt.Errorf("attestation failed with %d validation failures", len(*response.Results.Failures))
		}
		return "", fmt.Errorf("attestation validation failed: %s", string(failuresJSON))
	}

	return "", fmt.Errorf("attestation validation failed (no failure details provided)")
}
