#!/bin/bash
# Script to rebuild agent with code changes and update attestation policy
# This ensures measurements.txt matches the new agent binary

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=========================================="
echo "Rebuilding Agent & Updating Attestation Policy"
echo "=========================================="
echo ""

# Step 1: Build agent RPM with updated code
echo "Step 1: Building agent RPM..."
make rpm
echo "✓ RPM built successfully"
echo ""

# Step 2: Build agent disk image
echo "Step 2: Building agent disk image..."
# Force rebuild by removing old bundles and sentinel files
rm -f bin/.e2e-agent-images-*
rm -f bin/.e2e-agent-injected
rm -f bin/.e2e-agent-certs
rm -rf bin/agent-artifacts/
echo "Building agent disk image (this may take a while)..."
make e2e-agent-images
echo "✓ Agent disk image built successfully"
echo ""
echo "Preparing agent config with TPM attestation enabled..."
make prepare-agent-config-attestation
# Verify TPM config was added
if grep -q "tpm:" bin/agent/etc/flightctl/config.yaml; then
    echo "✓ TPM attestation enabled in agent config"
else
    echo "ERROR: TPM config not found in bin/agent/etc/flightctl/config.yaml"
    exit 1
fi
# Remove injection marker to force re-injection of the updated config
rm -f bin/.e2e-agent-injected
echo "✓ Removed injection marker - config will be re-injected when starting VM"
echo ""

# Step 3: Load bootc container image into podman
echo "Step 3: Loading bootc container image into podman..."
BUNDLE_TAR="bin/agent-artifacts/agent-images-bundle-cs9-bootc.tar"
if [ ! -f "$BUNDLE_TAR" ]; then
    echo "ERROR: Bundle tar not found at $BUNDLE_TAR"
    exit 1
fi

podman load -i "$BUNDLE_TAR"
echo "✓ Container image loaded into podman"
echo ""

# Step 4: Extract container image reference from manifest
echo "Step 4: Extracting container image reference..."
IMAGE_REF=$(tar -xOf "$BUNDLE_TAR" manifest.json | jq -r '.[0].RepoTags[0]')
if [ -z "$IMAGE_REF" ]; then
    echo "ERROR: Could not extract image reference from manifest"
    exit 1
fi
echo "Container image: $IMAGE_REF"
echo ""

# Step 5: Generate measurements from container image
echo "Step 5: Generating IMA measurements from container image..."
if ! command -v bootc2measurements &> /dev/null; then
    echo "ERROR: bootc2measurements not found in PATH"
    echo "Please install it first from: https://github.com/mpeters/bootc2measurements"
    exit 1
fi

bootc2measurements --image="$IMAGE_REF" --output_file=measurements.txt
MEASUREMENT_COUNT=$(wc -l < measurements.txt)
echo "✓ Generated $MEASUREMENT_COUNT IMA measurements"
echo ""

# Step 6: Generate attestation reference YAML
echo "Step 6: Generating attestation reference YAML..."
cd examples/attestation
./create-attestation-ref-from-measurements.sh ../../measurements.txt attestation-reference-from-measurements.yaml
cd "$SCRIPT_DIR"
echo "✓ Attestation reference YAML generated"
echo ""

# Step 7: Apply attestation reference to cluster
echo "Step 7: Applying attestation reference to FlightCTL..."
bin/flightctl apply -f examples/attestation/attestation-reference-from-measurements.yaml
echo "✓ Attestation reference applied"
echo ""

echo "=========================================="
echo "Rebuild Complete!"
echo "=========================================="
echo ""
echo "Summary:"
echo "  - Agent RPM rebuilt with updated code (empty PCR selection fix)"
echo "  - Agent disk image rebuilt at bin/output/qcow2/disk.qcow2"
echo "  - TPM attestation enabled in agent config (bin/agent/etc/flightctl/config.yaml)"
echo "  - Config marked for re-injection (will happen when starting VM)"
echo "  - Generated $MEASUREMENT_COUNT measurements from $IMAGE_REF"
echo "  - Attestation policy updated in FlightCTL"
echo ""
echo "Next steps:"
echo "  1. Clean old agent VM: make clean-agent-vm"
echo "  2. Start new agent VM: make agent-vm"
echo "     (This will inject the TPM-enabled config into the disk image)"
echo "  3. Monitor attestation: watch -n5 'bin/flightctl get enrollmentrequest'"
echo ""
echo "When the VM starts, the agent will:"
echo "  - Boot with TPM 2.0 emulator"
echo "  - Generate attestation data with empty PCR selection (no digest mismatch)"
echo "  - Send IMA measurements for verification"
echo "  - Attestation should succeed!"
echo ""
