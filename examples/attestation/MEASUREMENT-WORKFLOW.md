# IMA Measurement Extraction Workflow

This document describes how to extract IMA measurements for creating attestation policies.

## Overview

FlightCTL uses IMA (Integrity Measurement Architecture) runtime policies to verify that devices are running the expected software. The policy contains:
- **File digests**: SHA256 hashes of all files that should be present
- **Excludes**: Regex patterns for files that are allowed to change

## Two Measurement Sources

### 1. Container Image Measurements (bootc2measurements)

**Use case**: Quick policy generation from a container image

**Pros**:
- Fast and automated
- Doesn't require a running VM
- Reproducible from the same container image

**Cons**:
- Missing runtime-generated files (configs, caches, boot files)
- May have ~90-100 files that will fail verification
- Requires extensive exclude patterns

**Usage**:
```bash
# Extract measurements from container image
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
IMAGE="quay.io/flightctl/flightctl-device:v10-cs9-bootc"
bootc2measurements -i $IMAGE -o measurements-${TIMESTAMP}.txt

# Create attestation reference
./create-attestation-ref-from-measurements.sh measurements-${TIMESTAMP}.txt
```

### 2. Golden VM Measurements (Recommended for Production)

**Use case**: Production attestation policies

**Pros**:
- Includes all runtime-generated files
- Most accurate representation of running system
- Fewer exclude patterns needed
- Better security posture

**Cons**:
- Requires booting a "golden" VM first
- More manual process

**Usage**:
```bash
# 1. Build agent image
make AGENT_OS_ID=cs9-bootc e2e-agent-images

# 2. Boot a VM (don't enroll it yet)
make attestation-agent-vm

# 3. Wait for VM to fully boot and stabilize (~60 seconds)
sleep 60

# 4. Extract measurements from running VM
VM_IP=$(virsh -c qemu:///system domifaddr flightctl-device-default | \
        grep -oE '192\.168\.[0-9]+\.[0-9]+' | head -1)

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
ssh -o StrictHostKeyChecking=no core@${VM_IP} \
    'sudo cat /sys/kernel/security/ima/ascii_runtime_measurements' | \
    awk '{print $4, $5}' > measurements-golden-${TIMESTAMP}.txt

# 5. Create attestation reference
./create-attestation-ref-from-measurements.sh measurements-golden-${TIMESTAMP}.txt

# 6. Apply the policy BEFORE enrolling the device
bin/flightctl apply -f attestation-reference-from-measurements.yaml

# 7. Now clean the VM and create a fresh one for enrollment
make clean-agent-vm
make attestation-agent-vm

# 8. The fresh VM will be verified against the golden measurements
```

## Timestamped Files

All measurement files should be timestamped to track which agent build they correspond to:

**Format**: `measurements-YYYYMMDD-HHMMSS.txt`

**Example**: `measurements-20260311-132737.txt`

This timestamp should match:
- The disk image build time (for bootc2measurements)
- The golden VM boot time (for VM extraction)

## Exclude Patterns

The FlightCTL server automatically adds ~100 exclude patterns for:
- Temporary files and runtime state
- Lock files and caches
- Machine-specific files (machine-id, SSH keys)
- Boot-time generated files
- FlightCTL agent temporary files

See `internal/attestation/policy/converter.go` for the complete list.

## Best Practices

1. **Use timestamped filenames** to track measurement sources
2. **For production**: Extract from golden VM for highest accuracy
3. **For development**: bootc2measurements is faster but less accurate
4. **Version control**: Keep measurement files in git to track changes
5. **Test before deployment**: Verify policy works on a test device first
6. **Document changes**: Note when binaries are updated or configs change

## Troubleshooting

### Hash Mismatches

**Symptom**: `"Hash not found in runtime policy"` errors for specific files

**Cause**: The running VM has different file versions than the policy

**Solution**:
- Re-extract measurements from the current running VM
- Or rebuild the agent and extract fresh measurements

### Too Many Exclude Failures

**Symptom**: 50+ "File not found in allowlist" warnings

**Cause**: Using bootc2measurements without a golden VM baseline

**Solution**:
- Extract measurements from golden VM (recommended)
- Or add more exclude patterns for the failing files

### Stale Measurements

**Symptom**: Large number of hash mismatches after agent rebuild

**Cause**: Using measurement file from a previous build

**Solution**:
- Always re-extract measurements after rebuilding the agent
- Check file timestamps to ensure they match your current build
