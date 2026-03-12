package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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

	logrus.Infof("DetectFormat: content length = %d, first 100 chars: %.100s", len(trimmed), trimmed)

	// Try to parse as JSON first
	var testPolicy RuntimePolicy
	if err := json.Unmarshal([]byte(trimmed), &testPolicy); err == nil {
		logrus.Info("DetectFormat: parsed as JSON successfully")
		// Successfully parsed as JSON, verify it has the expected structure
		if testPolicy.Meta.Version >= 0 {
			logrus.Info("DetectFormat: detected as FormatJSON")
			return FormatJSON, nil
		}
	}
	logrus.Info("DetectFormat: not JSON, checking allowlist format")

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

// Default IMA runtime policy excludes
// These patterns match files that change between boots or deployments
var defaultExcludes = []string{
	// Boot aggregates (always change)
	"boot_aggregate",

	// Temporary and runtime files
	".*/tmp/.*",
	".*/\\.cache/.*",
	"/var/tmp/.*",
	"/run/.*",

	// Machine-specific system files
	".*/machine-id",
	".*/resolv\\.conf",
	".*/hostname",
	".*/localtime",

	// Dracut initramfs files
	".*/dracut/.*",
	".*/initramfs.*",

	// Container storage
	"/var/lib/containers/.*",
	".*/overlay/.*",
	".*/diff/.*",
	".*/merged/.*",

	// NetworkManager runtime state
	"/var/lib/NetworkManager/.*",
	"/etc/NetworkManager/system-connections/.*",

	// SSH host keys
	".*/ssh/ssh_host_.*",
	".*/\\.ssh/.*",

	// OSTree deployment files
	".*/ostree/deploy/.*/var/.*",
	".*/ostree/repo/.*",
	"/sysroot/ostree/.*",

	// Journal and log files
	".*/journal/.*",
	"/var/log/.*",

	// Hardware and driver database
	".*/hwdb\\.bin",
	".*/modules\\..*",

	// Grub configuration
	".*/grub.*",
	".*/grubenv",

	// SELinux policy
	".*/selinux/.*",
	".*/file_contexts.*",

	// PAM and authentication
	".*/pam\\.d/.*",

	// Systemd runtime state
	".*/systemd/.*\\.wants/.*",
	"/var/lib/systemd/.*",

	// Kernel modules
	".*/lib/modules/.*",

	// Certificates and crypto
	".*/pki/.*",
	".*/ssl/certs/.*",
	".*/ca-certificates/.*",

	// DNF/RPM database
	".*/dnf/.*",
	".*/rpm/.*",
	"/var/lib/rpm/.*",

	// Udev rules
	".*/udev/.*",

	// Firewall configuration
	".*/firewalld/.*",

	// Network configuration
	".*/sysconfig/network-scripts/.*",

	// Console and terminal
	".*/console/.*",

	// Greenboot health check
	".*/greenboot/.*",

	// RHSM subscription
	".*/rhsm/.*",

	// udisks
	".*/udisks2/.*",

	// Password and shadow files
	".*/passwd",
	".*/shadow",
	".*/group",
	".*/gshadow",

	// Config directories
	".*/conf\\.d/.*",

	// Kerberos libraries (change between boots)
	".*/libgssapi_krb5\\.so\\..*",
	".*/libk5crypto\\.so\\..*",
	".*/libkrb5\\.so\\..*",
	".*/libkrb5support\\.so\\..*",

	// Python binaries and libraries
	".*/python3\\.9",
	".*/libpython3\\.9\\.so\\..*",

	// Python bytecode cache
	".*/__pycache__/.*\\.pyc",

	// Python standard library modules
	".*/python3\\.9/.*\\.so",
	".*/python3\\.9/.*/__pycache__/.*",

	// System binaries that change
	".*/rpm-ostree",
	".*/grub2-editenv",
	".*/sshd",
	".*/skopeo",
	".*/setpriv",

	// FlightCTL banner files (temporary files with random suffixes)
	".*/issue\\.d/\\.flightctl-banner\\.issue.*",

	// Temporary network configuration files
	".*/resolv\\.conf\\..*",
	".*/NetworkManager/internal-.*",

	// bootc runtime storage
	"/run/bootc/.*",

	// OSTree repository refs
	"/sysroot/ostree/repo/refs/.*",

	// Lock files (created during file modifications)
	".*/\\.#.*",

	// FlightCTL agent temporary and state files
	"/var/lib/flightctl/.*",   // All FlightCTL state files (current.json, desired.json, etc.)
	"/var/lib/flightctl/certs/.*",

	// Runtime-generated configuration files (created at boot/first-run)
	// These files are not present in the container image and are generated
	// when the system boots or the agent first runs
	"/etc/adjtime",                    // System clock drift adjustment
	"/etc/chrony\\.conf",              // Chrony time sync config
	"/etc/chrony\\.keys",
	"/etc/environment",                // Environment variables
	"/etc/exports",                    // NFS exports
	"/etc/flightctl/config\\.yaml",   // FlightCTL agent config
	"/etc/host\\.conf",                // Hostname resolution order
	"/etc/hosts",                      // Static hostname mapping
	"/etc/idmapd\\.conf",              // NFSv4 ID mapping
	"/etc/issue",                      // Pre-login message
	"/etc/issue\\.d/.*",               // Issue message fragments
	"/etc/kdump\\.conf",               // Kernel crash dump config
	"/etc/ld\\.so\\.cache",            // Dynamic linker cache
	"/etc/ld\\.so\\.conf",             // Dynamic linker config
	"/etc/locale\\.conf",              // System locale
	"/etc/login\\.defs",               // Login configuration
	"/etc/lvm/.*",                     // LVM configuration
	"/etc/modprobe\\.d/.*",            // Kernel module config
	"/etc/containers/storage\\.conf", // Container storage config
	"/etc/dbus-1/.*",                  // D-Bus configuration
	"/etc/gss/.*",                     // GSS-API configuration
	"/etc/netconfig",                  // Network configuration
	"/etc/NetworkManager/.*",          // NetworkManager configs
	"/etc/nfs\\.conf",                 // NFS configuration
	"/etc/nsswitch\\.conf",            // Name service switch
	"/etc/protocols",                  // Network protocols
	"/etc/rpc",                        // RPC services
	"/etc/security/.*",                // Security configs (limits, namespace)
	"/etc/services",                   // Network services

	// Boot loader entries (generated per deployment)
	"/boot/loader\\..*",
	"/boot/loader/.*",

	// Dracut runtime files (initramfs generation and boot)
	"/dracut-state\\.sh",
	"/dracut/.*",
	".*/dracut-.*",              // Dracut binaries in /usr/bin
	".*/dracut.*\\.sh",          // Dracut libraries
	"/usr/lib/initrd-release",   // Initrd release info
	"/usr/lib/.*-lib\\.sh",      // Network and other boot libraries
	"/usr/lib/systemd/system-generators/dracut-.*",  // Dracut generators
	"/usr/lib/systemd/system/.*dracut.*",            // Dracut systemd units
	"/usr/sbin/initqueue",       // Dracut init queue

	// Systemd runtime configuration
	"/etc/systemd/.*",
	"/etc/sysctl\\.conf",
	"/etc/tmpfiles\\.d/.*",
	"/usr/lib/tmpfiles\\.d/.*",
	"/etc/sysconfig/.*",

	// Systemd unit files that change at runtime or are initrd-specific
	"/usr/lib/systemd/system/nm-.*initrd\\.service",      // NetworkManager initrd services (nm-initrd.service, nm-wait-online-initrd.service)
	"/usr/lib/systemd/system/systemd-tmpfiles-setup\\.service", // Modified by tmpfiles
	"/usr/lib/systemd/system/emergency\\.service",        // Emergency mode service
	"/usr/lib/systemd/system/dbus\\.socket",              // D-Bus socket activation

	// System caches
	"/var/cache/.*",

	// Filesystem and mount units
	"/sysroot/etc/fstab",

	// Additional system libraries and PAM modules
	"/usr/lib64/gssproxy/.*",
	"/usr/lib64/libcrack\\.so\\..*",
	"/usr/lib64/libgssrpc\\.so\\..*",
	"/usr/lib64/libpwquality\\.so\\..*",
	"/usr/lib64/security/pam_.*\\.so",
}

// LoadExcludes reads exclude patterns from a file
// The file should have one regex pattern per line, with # for comments
func LoadExcludes(excludesPath string) ([]string, error) {
	// If path doesn't exist, return default excludes
	if _, err := os.Stat(excludesPath); os.IsNotExist(err) {
		return defaultExcludes, nil
	}

	content, err := os.ReadFile(excludesPath)
	if err != nil {
		return defaultExcludes, nil
	}

	var excludes []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		excludes = append(excludes, line)
	}

	// If file is empty, return defaults
	if len(excludes) == 0 {
		return defaultExcludes, nil
	}

	return excludes, nil
}

// ParseAllowlist converts allowlist format to RuntimePolicy
// Input format: <hash> <filepath> per line
// Example: "abc123... /usr/bin/bash"
func ParseAllowlist(content string) (*RuntimePolicy, error) {
	logrus.Infof("ParseAllowlist: called with %d bytes of content", len(content))
	logrus.Infof("ParseAllowlist: defaultExcludes has %d patterns", len(defaultExcludes))
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

	// Use default excludes embedded in the code
	// These can be overridden by reading from a file in the future
	excludes := defaultExcludes
	logrus.Infof("ParseAllowlist: setting excludes to defaultExcludes (%d patterns)", len(excludes))

	policy := &RuntimePolicy{
		Meta: Meta{
			Version:   1,
			Generator: GeneratorCompatibleList, // Use Keylime's CompatibleList generator (2) - trying different value
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Release:  0,
		Digests:  digests,
		Excludes: excludes,
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
	logrus.Infof("ConvertToJSON: called with %d bytes of content", len(content))

	format, err := DetectFormat(content)
	if err != nil {
		logrus.Errorf("ConvertToJSON: failed to detect format: %v", err)
		return "", fmt.Errorf("failed to detect format: %w", err)
	}

	logrus.Infof("ConvertToJSON: detected format = %v", format)

	var policy *RuntimePolicy

	switch format {
	case FormatJSON:
		// Already JSON, just validate it
		logrus.Info("ConvertToJSON: parsing as JSON")
		policy = &RuntimePolicy{}
		if err := json.Unmarshal([]byte(content), policy); err != nil {
			return "", fmt.Errorf("failed to parse JSON: %w", err)
		}
		logrus.Infof("ConvertToJSON: JSON policy has %d excludes", len(policy.Excludes))

	case FormatAllowlist:
		// Convert from allowlist
		logrus.Info("ConvertToJSON: converting from allowlist format")
		policy, err = ParseAllowlist(content)
		if err != nil {
			return "", fmt.Errorf("failed to parse allowlist: %w", err)
		}
		logrus.Infof("ConvertToJSON: converted allowlist has %d excludes", len(policy.Excludes))

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

	logrus.Infof("ConvertToJSON: successfully converted, final JSON has %d excludes", len(policy.Excludes))
	return string(jsonBytes), nil
}
