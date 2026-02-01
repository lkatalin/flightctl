package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: log,
	}
}

// VerifyAttestation sends TPM attestation data to the Keylime verifier for verification.
// This is a blocking call that waits for the verifier to complete verification.
// Returns the verification result status string ("Success" or error details) and any error.
func (c *Client) VerifyAttestation(ctx context.Context, deviceID string, aikTpm string, ekTpm *string, mbPolicy, runtimePolicy, tpmPolicy *string) (string, error) {
	url := fmt.Sprintf("%s/v2.1/agents/%s", c.baseURL, deviceID)

	req := keylimeapi.AttestationVerificationRequest{
		AikTpm: aikTpm,
	}

	if ekTpm != nil {
		req.EkTpm = ekTpm
	}
	if mbPolicy != nil {
		req.MbPolicy = mbPolicy
	}
	if runtimePolicy != nil {
		req.RuntimePolicy = runtimePolicy
	}
	if tpmPolicy != nil {
		req.TpmPolicy = tpmPolicy
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	c.log.Debugf("Verifying attestation for device %s with Keylime verifier at %s", deviceID, url)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp keylimeapi.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			statusMsg := "unknown"
			if errResp.Status != nil {
				statusMsg = *errResp.Status
			}
			return "", fmt.Errorf("keylime verifier returned error (status %d): %s", resp.StatusCode, statusMsg)
		}
		return "", fmt.Errorf("keylime verifier returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var response keylimeapi.AttestationVerificationResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.log.Infof("Keylime verifier completed attestation verification for device %s: %s", deviceID, response.Results)
	return response.Results, nil
}
