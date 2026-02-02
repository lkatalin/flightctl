package policy

// RuntimePolicy represents a Keylime IMA runtime policy structure
// Based on Keylime's RUNTIME_POLICY_SCHEMA
type RuntimePolicy struct {
	Meta             Meta                `json:"meta"`
	Release          int                 `json:"release"`
	Digests          map[string][]string `json:"digests"`
	Excludes         []string            `json:"excludes"`
	Keyrings         map[string][]string `json:"keyrings"`
	IMA              IMAConfig           `json:"ima"`
	IMABuf           map[string][]string `json:"ima-buf"`
	VerificationKeys string              `json:"verification-keys"`
}

// Meta contains metadata about the runtime policy
type Meta struct {
	Version   int    `json:"version"`
	Generator int    `json:"generator"`
	Timestamp string `json:"timestamp,omitempty"`
}

// IMAConfig contains IMA-specific configuration
type IMAConfig struct {
	IgnoredKeyrings []string `json:"ignored_keyrings"`
	LogHashAlg      string   `json:"log_hash_alg"`
	DMPolicy        *string  `json:"dm_policy"`
}

// Generator enum values - must match Keylime's RUNTIME_POLICY_GENERATOR enum
const (
	GeneratorUnknown         = 0
	GeneratorEmptyAllowList  = 1
	GeneratorCompatibleList  = 2
	GeneratorLegacyAllowList = 3
	// Note: Keylime only recognizes values 0-3. Do not add custom values beyond 3.
)

// FormatType represents the detected format of a runtime policy
type FormatType int

const (
	FormatUnknown   FormatType = iota
	FormatJSON                 // Already in Keylime JSON format
	FormatAllowlist            // Plain text allowlist format (hash filepath)
)
