#!/bin/bash
# extract-bootc-measurements.sh
# Extracts IMA measurements from bootc container image and creates attestation reference
#
# This script:
# 1. Loads the bootc container bundle into podman
# 2. Extracts the container image reference
# 3. Uses bootc2measurements to generate IMA measurements
# 4. Converts to AttestationReference YAML
# 5. Applies it to the server
#
# This approach is faster than booting a VM and extracting via SSH

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
MEASUREMENTS_FILE="measurements-${TIMESTAMP}.txt"
ATTESTATION_REF_FILE="attestation-reference-${TIMESTAMP}.yaml"
BUNDLE_TAR="bin/agent-artifacts/agent-images-bundle-cs9-bootc.tar"

echo "==========================================="
echo "Extract Measurements from bootc Container"
echo "==========================================="
echo ""
echo "Timestamp: $TIMESTAMP"
echo "Measurements file: $MEASUREMENTS_FILE"
echo "Attestation reference: $ATTESTATION_REF_FILE"
echo ""

# Step 1: Check for bootc2measurements tool
if ! command -v bootc2measurements &> /dev/null; then
    echo "ERROR: bootc2measurements not found in PATH"
    echo "Please install it first from: https://github.com/mpeters/bootc2measurements"
    exit 1
fi
echo "✓ bootc2measurements found"
echo ""

# Step 2: Check for bundle tar
echo "Step 1: Checking for bootc container bundle..."
if [ ! -f "$BUNDLE_TAR" ]; then
    echo "ERROR: Bundle tar not found at $BUNDLE_TAR"
    echo "Run 'make e2e-agent-images' first to build the agent disk image"
    exit 1
fi
echo "✓ Found bundle: $BUNDLE_TAR"
echo ""

# Step 3: Load bootc container image into podman
echo "Step 2: Loading bootc container image into podman..."
podman load -i "$BUNDLE_TAR" >/dev/null 2>&1
echo "✓ Container image loaded into podman"
echo ""

# Step 4: Extract container image reference from manifest
echo "Step 3: Extracting container image reference..."
IMAGE_REF=$(tar -xOf "$BUNDLE_TAR" manifest.json | jq -r '.[0].RepoTags[0]')
if [ -z "$IMAGE_REF" ]; then
    echo "ERROR: Could not extract image reference from manifest"
    exit 1
fi
echo "Container image: $IMAGE_REF"
echo ""

# Step 5: Generate measurements from container image
echo "Step 4: Generating IMA measurements from container image..."
bootc2measurements --image="$IMAGE_REF" --output_file="$MEASUREMENTS_FILE"
MEASUREMENT_COUNT=$(wc -l < "$MEASUREMENTS_FILE")
echo "✓ Generated $MEASUREMENT_COUNT IMA measurements"
echo ""

# Step 6: Create AttestationReference YAML
echo "Step 5: Creating AttestationReference YAML..."
cd examples/attestation
./create-attestation-ref-from-measurements.sh "../../$MEASUREMENTS_FILE" "../../$ATTESTATION_REF_FILE"
cd "$SCRIPT_DIR"
echo ""

# Step 7: Apply to server
echo "Step 6: Applying AttestationReference to server..."
bin/flightctl apply -f "$ATTESTATION_REF_FILE"

echo ""
echo "==========================================="
echo "Measurement Extraction Complete!"
echo "==========================================="
echo ""
echo "Created files:"
echo "  - $MEASUREMENTS_FILE ($MEASUREMENT_COUNT measurements)"
echo "  - $ATTESTATION_REF_FILE"
echo ""
echo "✓ AttestationReference 'flightctl-baseline-policy' applied to server"
echo ""
echo "Measurements extracted from bootc container image: $IMAGE_REF"
echo ""
echo "Next steps:"
echo "  1. Start agent VM: make attestation-agent-vm"
echo "  2. Monitor enrollment: watch -n5 'bin/flightctl get enrollmentrequest'"
echo ""
