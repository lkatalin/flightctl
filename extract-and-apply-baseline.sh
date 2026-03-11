#!/bin/bash
# extract-and-apply-baseline.sh
# Extracts IMA measurements from enrollment request and creates attestation reference
#
# This script:
# 1. Waits for an enrollment request to appear
# 2. Extracts IMA measurements from it
# 3. Converts to allowlist format with timestamp
# 4. Creates an AttestationReference YAML
# 5. Applies it to the server

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
MEASUREMENTS_FILE="measurements-${TIMESTAMP}.txt"
ATTESTATION_REF_FILE="attestation-reference-${TIMESTAMP}.yaml"

echo "=========================================="
echo "Extract and Apply Baseline Workflow"
echo "=========================================="
echo ""
echo "Timestamp: $TIMESTAMP"
echo "Measurements file: $MEASUREMENTS_FILE"
echo "Attestation reference: $ATTESTATION_REF_FILE"
echo ""

# Step 1: Wait for enrollment request
echo "Step 1: Waiting for enrollment request (max 60 seconds)..."
TIMEOUT=60
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    COUNT=$(bin/flightctl get enrollmentrequests -o json 2>/dev/null | jq -r '.items | length' || echo 0)
    if [ "$COUNT" -gt 0 ]; then
        echo "✓ Found $COUNT enrollment request(s)"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
    printf "."
done
echo ""

if [ "$COUNT" -eq 0 ]; then
    echo "ERROR: No enrollment request found after ${TIMEOUT}s"
    exit 1
fi

# Step 2: Extract IMA measurements from most recent enrollment
echo ""
echo "Step 2: Extracting IMA measurements from enrollment request..."
ENROLLMENT_NAME=$(bin/flightctl get enrollmentrequests -o json | jq -r '.items | sort_by(.metadata.creationTimestamp) | .[-1] | .metadata.name')
echo "Using enrollment: $ENROLLMENT_NAME"

bin/flightctl get enrollmentrequest "$ENROLLMENT_NAME" -o json | \
    jq -r '.spec.attestationData.imaMeasurementList' > /tmp/raw-ima-log.txt

if [ ! -s /tmp/raw-ima-log.txt ]; then
    echo "ERROR: No IMA measurements found in enrollment request"
    exit 1
fi

RAW_LINES=$(wc -l < /tmp/raw-ima-log.txt)
echo "✓ Extracted $RAW_LINES lines of IMA measurements"

# Step 3: Convert to allowlist format
echo ""
echo "Step 3: Converting to allowlist format..."
echo "  - Filtering out boot_aggregate"
echo "  - Filtering out all-zero and all-FF hashes (unmeasurable files)"
echo "  - Converting to 'hash filepath' format"

cat /tmp/raw-ima-log.txt | \
    grep -v "0000000000000000000000000000000000000000000000000000000000000000" | \
    grep -v "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" | \
    awk 'NF >= 4 {print $(NF-1), $NF}' | \
    sed 's/sha256://' | \
    sed 's/sha1://' | \
    grep -v " boot_aggregate$" | \
    grep -v "^sha256: " | \
    grep -v "^sha1: " > "$MEASUREMENTS_FILE"

MEASUREMENT_COUNT=$(wc -l < "$MEASUREMENTS_FILE")
echo "✓ Created $MEASUREMENTS_FILE with $MEASUREMENT_COUNT measurements"

# Step 4: Create AttestationReference YAML
echo ""
echo "Step 4: Creating AttestationReference YAML..."
cd examples/attestation
./create-attestation-ref-from-measurements.sh "../../$MEASUREMENTS_FILE" "../../$ATTESTATION_REF_FILE"
cd "$SCRIPT_DIR"

# Step 5: Apply to server
echo ""
echo "Step 5: Applying AttestationReference to server..."
bin/flightctl apply -f "$ATTESTATION_REF_FILE"

echo ""
echo "=========================================="
echo "Baseline Extraction Complete!"
echo "=========================================="
echo ""
echo "Created files:"
echo "  - $MEASUREMENTS_FILE ($MEASUREMENT_COUNT measurements)"
echo "  - $ATTESTATION_REF_FILE"
echo ""
echo "✓ AttestationReference 'flightctl-baseline-policy' applied to server"
echo ""
echo "The baseline was extracted from enrollment request: $ENROLLMENT_NAME"
echo ""
