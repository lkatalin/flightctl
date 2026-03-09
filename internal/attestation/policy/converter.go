package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// Regex to match allowlist format: hash (whitespace) filepath
	// Hash is 40-128 hex characters (sha1=40, sha256=64, sha384=96, sha512=128)
	allowlistLineRegex = regexp.MustCompile(`^([0-9a-fA-F]{40,128})\s+(.+)$`)
)

// DetectFormat attempts to determine if the content is JSON or allowlist format
func DetectFormat(content string) (FormatType, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return FormatUnknown, fmt.Errorf("empty content")
	}

	// Try to parse as JSON first
	var testPolicy RuntimePolicy
	if err := json.Unmarshal([]byte(trimmed), &testPolicy); err == nil {
		// Successfully parsed as JSON, verify it has the expected structure
		if testPolicy.Meta.Version >= 0 {
			return FormatJSON, nil
		}
	}

	// Check if it looks like allowlist format
	// Should have lines matching: hash filepath
	lines := strings.Split(trimmed, "\n")
	matchCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}
		if allowlistLineRegex.MatchString(line) {
			matchCount++
		}
		if matchCount >= 3 {
			// If we have at least 3 valid lines, assume it's allowlist format
			return FormatAllowlist, nil
		}
	}

	if matchCount > 0 {
		// Has some valid lines but maybe not 3 yet
		return FormatAllowlist, nil
	}

	return FormatUnknown, fmt.Errorf("unable to determine format: not valid JSON or allowlist")
}

// ParseAllowlist converts allowlist format to RuntimePolicy
// Input format: <hash> <filepath> per line
// Example: "abc123... /usr/bin/bash"
func ParseAllowlist(content string) (*RuntimePolicy, error) {
	lines := strings.Split(content, "\n")
	digests := make(map[string][]string)

	lineNum := 0
	for _, line := range lines {
		lineNum++
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := allowlistLineRegex.FindStringSubmatch(line)
		if matches == nil {
			return nil, fmt.Errorf("invalid allowlist format at line %d: %s", lineNum, line)
		}

		hash := strings.ToLower(matches[1])
		filepath := matches[2]

		// Validate hash length (40 for sha1, 64 for sha256, 96 for sha384, 128 for sha512)
		hashLen := len(hash)
		if hashLen != 40 && hashLen != 64 && hashLen != 96 && hashLen != 128 {
			return nil, fmt.Errorf("invalid hash length at line %d: expected 40/64/96/128, got %d", lineNum, hashLen)
		}

		// Add to digests map (filepath -> list of allowed hashes)
		digests[filepath] = append(digests[filepath], hash)
	}

	if len(digests) == 0 {
		return nil, fmt.Errorf("no valid digest entries found in allowlist")
	}

	// NOTE: We detect the FILE CONTENT hash algorithm (sha1, sha256, etc.) from the
	// hash length in the measurements, but this is different from the IMA template hash
	// algorithm. File content hashes can be SHA256, SHA512, etc., but IMA template hashes
	// are always SHA1 for standard ima-ng format. The file hash algorithm is embedded in
	// the IMA template data but doesn't affect the template hash calculation itself.

	// IMA template hashes are ALWAYS SHA1 for standard ima-ng format,
	// regardless of the file content hash algorithm.
	// The ima.log_hash_alg setting tells Keylime what hash algorithm
	// the IMA log uses for TEMPLATE hashes (not file content hashes).
	// For ima-ng templates, this is always sha1.
	templateHashAlg := "sha1"

	policy := &RuntimePolicy{
		Meta: Meta{
			Version:   1,
			Generator: GeneratorCompatibleList, // Use Keylime's CompatibleList generator (2) - trying different value
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Release:  0,
		Digests:  digests,
		Excludes: []string{},
		Keyrings: make(map[string][]string),
		IMA: IMAConfig{
			IgnoredKeyrings: []string{},
			LogHashAlg:      templateHashAlg, // Always sha1 for ima-ng template hashes
			DMPolicy:        nil,
		},
		IMABuf:           make(map[string][]string),
		VerificationKeys: "",
	}

	return policy, nil
}

// ValidateRuntimePolicy validates a RuntimePolicy structure
func ValidateRuntimePolicy(policy *RuntimePolicy) error {
	if policy == nil {
		return fmt.Errorf("policy is nil")
	}

	if policy.Meta.Version < 0 {
		return fmt.Errorf("invalid meta.version: must be >= 0")
	}

	if policy.Meta.Generator < 0 {
		return fmt.Errorf("invalid meta.generator: must be >= 0")
	}

	// Validate hash algorithm
	validHashAlgs := map[string]bool{
		"sha1":   true,
		"sha256": true,
		"sha384": true,
		"sha512": true,
	}
	if !validHashAlgs[policy.IMA.LogHashAlg] {
		return fmt.Errorf("invalid ima.log_hash_alg: must be sha1, sha256, sha384, or sha512")
	}

	// Validate digest hashes are lowercase hex
	hexRegex := regexp.MustCompile(`^[0-9a-f]+$`)
	for path, hashes := range policy.Digests {
		if len(hashes) == 0 {
			return fmt.Errorf("digest entry for %s has no hashes", path)
		}
		for _, hash := range hashes {
			hashLen := len(hash)
			if hashLen != 40 && hashLen != 64 && hashLen != 96 && hashLen != 128 {
				return fmt.Errorf("invalid hash length for %s: %d", path, hashLen)
			}
			if !hexRegex.MatchString(hash) {
				return fmt.Errorf("invalid hash format for %s: must be lowercase hex", path)
			}
		}
	}

	return nil
}

// ConvertToJSON converts any supported format to JSON string
func ConvertToJSON(content string) (string, error) {
	format, err := DetectFormat(content)
	if err != nil {
		return "", fmt.Errorf("failed to detect format: %w", err)
	}

	var policy *RuntimePolicy

	switch format {
	case FormatJSON:
		// Already JSON, just validate it
		policy = &RuntimePolicy{}
		if err := json.Unmarshal([]byte(content), policy); err != nil {
			return "", fmt.Errorf("failed to parse JSON: %w", err)
		}

	case FormatAllowlist:
		// Convert from allowlist
		policy, err = ParseAllowlist(content)
		if err != nil {
			return "", fmt.Errorf("failed to parse allowlist: %w", err)
		}

	default:
		return "", fmt.Errorf("unknown format")
	}

	// Validate the policy
	if err := ValidateRuntimePolicy(policy); err != nil {
		return "", fmt.Errorf("policy validation failed: %w", err)
	}

	// Convert to JSON
	jsonBytes, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	return string(jsonBytes), nil
}
