# TPM Attestation with Keylime Verifier

This directory contains examples and documentation for using TPM attestation with FlightCTL and Keylime.

## Overview

FlightCTL supports TPM-based remote attestation using the Keylime verifier. During device enrollment, the agent can send TPM measurements (IMA runtime measurements, measured boot logs, TPM quotes) which are verified against an AttestationReference policy.

## Architecture

```
Agent (with TPM) → Enrollment Request + Attestation Data
                ↓
FlightCTL API Server → Looks up AttestationReference
                ↓
Keylime Verifier ← Sends attestation data + policy
                ↓
Verification Result → Enrollment Approved/Denied
```

## Modes

### Mock Mode (Default - Development Only)
- `keylime.enabled: false` in Helm values
- **NO verification performed** - all attestations automatically succeed
- Logs **WARNING** messages indicating insecure mode
- **DO NOT USE IN PRODUCTION**

### Real Mode (Production)
- `keylime.enabled: true` in Helm values
- Deploys official Keylime verifier container
- Performs actual cryptographic verification
- Only allows enrollment if attestation data matches policy

## Runtime Policy Formats

FlightCTL accepts runtime policies in **two formats** - both are automatically converted to Keylime's JSON format:

### 1. Allowlist Format (Simple Text)

The allowlist format is a simple text file with one measurement per line:

```
<sha256_hash> <filepath>
<sha256_hash> <filepath>
...
```

Example:
```
abc123def456abc123def456abc123def456abc123def456abc123def456abc1 /usr/bin/bash
def456abc123def456abc123def456abc123def456abc123def456abc123def4 /usr/bin/ls
```

See: `attestation-reference-allowlist.yaml`

### 2. Keylime JSON Format

The full Keylime runtime policy JSON structure:

```json
{
  "meta": {"version": 1, "generator": 4},
  "release": 0,
  "digests": {
    "/usr/bin/bash": ["abc123..."],
    "/usr/bin/ls": ["def456..."]
  },
  "excludes": [],
  "keyrings": {},
  "ima": {
    "ignored_keyrings": [],
    "log_hash_alg": "sha256",
    "dm_policy": null
  },
  "ima-buf": {},
  "verification-keys": ""
}
```

See: `attestation-reference-json.yaml`

**Both formats work identically** - FlightCTL auto-detects and converts allowlist to JSON.

## Using measurements.txt

The `measurements.txt` file in the repository root contains ~88,500 IMA measurements from a FlightCTL agent system running the container image `quay.io/lsturman/centos-bootc-flightctl-18nov2025`. This represents a "golden baseline" of expected file measurements.

**Important:** For attestation verification to succeed, the agent VM must run the exact same container image that was used to generate measurements.txt. The `make attestation-demo` target automatically uses this image when building the agent VM.

### To use measurements.txt as a runtime policy:

1. **Create AttestationReference with measurements.txt content:**

```yaml
apiVersion: v1beta1
kind: AttestationReference
metadata:
  name: flightctl-baseline-policy
spec:
  matchAll: true
  runtimePolicy: |
    <paste entire contents of measurements.txt here>
```

2. **Or use kubectl to create from file:**

```bash
# Create YAML with measurements.txt embedded
cat > attestation-ref.yaml <<EOF
apiVersion: v1beta1
kind: AttestationReference
metadata:
  name: flightctl-baseline-policy
spec:
  matchAll: true
  runtimePolicy: |
$(cat ../../measurements.txt | sed 's/^/    /')
EOF

# Apply it
kubectl apply -f attestation-ref.yaml
```

3. **FlightCTL will automatically convert** the allowlist format to Keylime JSON format.

## Quick Start: Attestation Demo

**Prerequisites:**
- Libvirt virtualization tools (Linux host only):
  - Fedora/RHEL: `sudo dnf install virt-install libvirt qemu-kvm && sudo systemctl enable --now libvirtd`
  - Ubuntu/Debian: `sudo apt-get install virt-manager libvirt-daemon-system qemu-kvm && sudo systemctl enable --now libvirtd`
  - Add your user to libvirt group: `sudo usermod -aG libvirt $USER` (requires logout/login)

Run the complete attestation demo with one command:

```bash
make attestation-demo
```

This will:
1. Build all FlightCTL containers
2. Deploy to kind cluster with Keylime enabled
3. Start official Keylime verifier container
4. Create TPM-enabled agent VM with vTPM 2.0 emulator using `quay.io/lsturman/centos-bootc-flightctl-18nov2025`
5. Enable end-to-end attestation verification

The demo runs in **real mode** with actual Keylime verification. The agent VM uses the same container image that was used to generate `measurements.txt`, ensuring attestation verification will succeed when the baseline policy is applied.

## Manual Setup

### 1. Deploy FlightCTL with Keylime enabled

```bash
# Deploy with real Keylime verifier
helm upgrade --install flightctl ./deploy/helm/flightctl \
  --set keylime.enabled=true \
  --values ./deploy/helm/flightctl/values.dev.yaml
```

### 2. Create AttestationReference

Apply one of the example policies:

```bash
# Using allowlist format (simpler)
kubectl apply -f examples/attestation/attestation-reference-allowlist.yaml

# Or using JSON format
kubectl apply -f examples/attestation/attestation-reference-json.yaml
```

### 3. Enroll Agent with TPM

The agent must be configured to send attestation data during enrollment. This requires:
- TPM 2.0 hardware or emulator (swtpm)
- Agent built with TPM support
- Attestation data collection enabled

When the agent enrolls, FlightCTL will:
1. Extract attestation data from the enrollment request
2. Look up the matching AttestationReference (via `matchAll: true` or labels)
3. Send attestation data + policy to Keylime verifier
4. Approve enrollment only if verification succeeds

## Verification

Check attestation status in the EnrollmentRequest:

```bash
kubectl get enrollmentrequest <name> -o yaml
```

Look for conditions:
```yaml
status:
  conditions:
  - type: AttestationVerified
    status: "True"
    reason: "AttestationVerificationSucceeded"
    message: "Keylime verifier verified attestation successfully"
```

Or in mock mode:
```yaml
status:
  conditions:
  - type: AttestationVerified
    status: "True"
    reason: "AttestationMockMode"
    message: "WARNING: Attestation verification skipped (mock mode - Keylime not enabled)"
```

## Hash Algorithms

The policy converter supports multiple hash algorithms:
- SHA-1 (40 hex characters)
- SHA-256 (64 hex characters) - **recommended**
- SHA-384 (96 hex characters)
- SHA-512 (128 hex characters)

The algorithm is auto-detected from hash length.

## Troubleshooting

### Keylime verifier not starting
```bash
kubectl logs -n flightctl-external deployment/keylime-verifier
kubectl describe pod -n flightctl-external -l flightctl.service=keylime-verifier
```

### Attestation verification failing
Check the EnrollmentRequest status and FlightCTL API logs:
```bash
kubectl logs -n flightctl-external deployment/flightctl-api | grep -i attestation
```

### Mock mode warnings
If you see warnings about mock mode but want real verification:
```bash
helm upgrade flightctl ./deploy/helm/flightctl --reuse-values --set keylime.enabled=true
```

## Security Considerations

⚠️ **IMPORTANT**: Mock mode bypasses all attestation verification. Only use it for:
- Local development
- CI/CD testing
- Environments where attestation is not required

For production deployments:
- Always use `keylime.enabled: true`
- Use comprehensive runtime policies (like measurements.txt)
- Regularly update policies as system images change
- Monitor attestation verification failures

## References

- [Keylime Project](https://keylime.dev/)
- [IMA/EVM Documentation](https://sourceforge.net/p/linux-ima/wiki/Home/)
- [TPM 2.0 Specification](https://trustedcomputinggroup.org/resource/tpm-library-specification/)
