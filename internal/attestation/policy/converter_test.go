package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected FormatType
		wantErr  bool
	}{
		{
			name:     "Empty content",
			content:  "",
			expected: FormatUnknown,
			wantErr:  true,
		},
		{
			name: "Valid JSON format",
			content: `{
				"meta": {"version": 1, "generator": 0},
				"release": 0,
				"digests": {},
				"excludes": [],
				"keyrings": {},
				"ima": {"ignored_keyrings": [], "log_hash_alg": "sha256", "dm_policy": null},
				"ima-buf": {},
				"verification-keys": ""
			}`,
			expected: FormatJSON,
			wantErr:  false,
		},
		{
			name: "Valid allowlist format",
			content: `abc123def456abc123def456abc123def456abc123def456abc123def456abc1 /usr/bin/bash
def456abc123def456abc123def456abc123def456abc123def456abc123def456 /usr/bin/ls
1234567890123456789012345678901234567890123456789012345678901234 /etc/passwd`,
			expected: FormatAllowlist,
			wantErr:  false,
		},
		{
			name: "Allowlist with comments and empty lines",
			content: `# This is a comment
abc123def456abc123def456abc123def456abc123def456abc123def456abc1 /usr/bin/bash

def456abc123def456abc123def456abc123def456abc123def456abc123def456 /usr/bin/ls`,
			expected: FormatAllowlist,
			wantErr:  false,
		},
		{
			name:     "Invalid format",
			content:  "this is not a valid format",
			expected: FormatUnknown,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectFormat(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("DetectFormat() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseAllowlist(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantErr      bool
		expectedAlg  string
		expectedSize int
	}{
		{
			name: "Valid SHA256 allowlist",
			content: `abc123def456abc123def456abc123def456abc123def456abc123def456abc1 /usr/bin/bash
def456abc123def456abc123def456abc123def456abc123def456abc123def4 /usr/bin/ls`,
			wantErr:      false,
			expectedAlg:  "sha256",
			expectedSize: 2,
		},
		{
			name: "Valid SHA1 allowlist",
			content: `abc123def456abc123def456abc123def456abc1 /usr/bin/bash
def456abc123def456abc123def456abc123def4 /usr/bin/ls`,
			wantErr:      false,
			expectedAlg:  "sha1",
			expectedSize: 2,
		},
		{
			name: "Allowlist with spaces in filepath",
			content: `abc123def456abc123def456abc123def456abc123def456abc123def456abc1 /usr/local/my folder/file.txt
def456abc123def456abc123def456abc123def456abc123def456abc123def4 /etc/test file`,
			wantErr:      false,
			expectedAlg:  "sha256",
			expectedSize: 2,
		},
		{
			name:    "Invalid hash length",
			content: `abc123 /usr/bin/bash`,
			wantErr: true,
		},
		{
			name:    "Invalid format - no filepath",
			content: `abc123def456abc123def456abc123def456abc123def456abc123def456abc12345`,
			wantErr: true,
		},
		{
			name:    "Empty allowlist",
			content: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := ParseAllowlist(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAllowlist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if policy.IMA.LogHashAlg != tt.expectedAlg {
				t.Errorf("ParseAllowlist() hash alg = %v, want %v", policy.IMA.LogHashAlg, tt.expectedAlg)
			}

			if len(policy.Digests) != tt.expectedSize {
				t.Errorf("ParseAllowlist() digest count = %v, want %v", len(policy.Digests), tt.expectedSize)
			}

			// Verify hashes are lowercase
			for _, hashes := range policy.Digests {
				for _, hash := range hashes {
					if hash != strings.ToLower(hash) {
						t.Errorf("ParseAllowlist() hash not lowercase: %v", hash)
					}
				}
			}
		})
	}
}

func TestValidateRuntimePolicy(t *testing.T) {
	validPolicy := &RuntimePolicy{
		Meta: Meta{
			Version:   1,
			Generator: GeneratorLegacyAllowList,
		},
		Release: 0,
		Digests: map[string][]string{
			"/usr/bin/bash": {"abc123def456abc123def456abc123def456abc123def456abc123def456abc1"},
		},
		Excludes: []string{},
		Keyrings: make(map[string][]string),
		IMA: IMAConfig{
			IgnoredKeyrings: []string{},
			LogHashAlg:      "sha256",
			DMPolicy:        nil,
		},
		IMABuf:           make(map[string][]string),
		VerificationKeys: "",
	}

	tests := []struct {
		name    string
		policy  *RuntimePolicy
		wantErr bool
	}{
		{
			name:    "Valid policy",
			policy:  validPolicy,
			wantErr: false,
		},
		{
			name:    "Nil policy",
			policy:  nil,
			wantErr: true,
		},
		{
			name: "Invalid hash algorithm",
			policy: &RuntimePolicy{
				Meta:     Meta{Version: 1, Generator: 0},
				Release:  0,
				Digests:  map[string][]string{},
				Excludes: []string{},
				Keyrings: make(map[string][]string),
				IMA: IMAConfig{
					IgnoredKeyrings: []string{},
					LogHashAlg:      "md5",
					DMPolicy:        nil,
				},
				IMABuf:           make(map[string][]string),
				VerificationKeys: "",
			},
			wantErr: true,
		},
		{
			name: "Invalid hash format - uppercase",
			policy: &RuntimePolicy{
				Meta:    Meta{Version: 1, Generator: 0},
				Release: 0,
				Digests: map[string][]string{
					"/usr/bin/bash": {"ABC123DEF456ABC123DEF456ABC123DEF456ABC123DEF456ABC123DEF456ABC1"},
				},
				Excludes: []string{},
				Keyrings: make(map[string][]string),
				IMA: IMAConfig{
					IgnoredKeyrings: []string{},
					LogHashAlg:      "sha256",
					DMPolicy:        nil,
				},
				IMABuf:           make(map[string][]string),
				VerificationKeys: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuntimePolicy(tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRuntimePolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConvertToJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "Convert allowlist to JSON",
			content: `abc123def456abc123def456abc123def456abc123def456abc123def456abc1 /usr/bin/bash
def456abc123def456abc123def456abc123def456abc123def456abc123def4 /usr/bin/ls`,
			wantErr: false,
		},
		{
			name: "Pass through valid JSON",
			content: `{
				"meta": {"version": 1, "generator": 0},
				"release": 0,
				"digests": {"/usr/bin/bash": ["abc123def456abc123def456abc123def456abc123def456abc123def456abc1"]},
				"excludes": [],
				"keyrings": {},
				"ima": {"ignored_keyrings": [], "log_hash_alg": "sha256", "dm_policy": null},
				"ima-buf": {},
				"verification-keys": ""
			}`,
			wantErr: false,
		},
		{
			name:    "Invalid content",
			content: "not valid at all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonStr, err := ConvertToJSON(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Verify output is valid JSON
			var policy RuntimePolicy
			if err := json.Unmarshal([]byte(jsonStr), &policy); err != nil {
				t.Errorf("ConvertToJSON() produced invalid JSON: %v", err)
			}

			// Verify it validates
			if err := ValidateRuntimePolicy(&policy); err != nil {
				t.Errorf("ConvertToJSON() produced invalid policy: %v", err)
			}
		})
	}
}
