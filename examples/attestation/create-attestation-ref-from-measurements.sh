#!/bin/bash
# Helper script to create an AttestationReference from measurements.txt
# Usage: ./create-attestation-ref-from-measurements.sh [measurements-file] [output-file]

set -euo pipefail

MEASUREMENTS_FILE="${1:-../../measurements.txt}"
OUTPUT_FILE="${2:-attestation-reference-from-measurements.yaml}"

if [ ! -f "$MEASUREMENTS_FILE" ]; then
    echo "Error: Measurements file not found: $MEASUREMENTS_FILE"
    echo "Usage: $0 [measurements-file] [output-file]"
    exit 1
fi

echo "Creating AttestationReference from $MEASUREMENTS_FILE..."
echo "Output: $OUTPUT_FILE"

# Count measurements
COUNT=$(wc -l < "$MEASUREMENTS_FILE")
echo "Found $COUNT measurements"

# Create YAML with embedded measurements
cat > "$OUTPUT_FILE" <<EOF
apiVersion: v1beta1
kind: AttestationReference
metadata:
  name: flightctl-baseline-policy
  labels:
    app: flightctl
    policy-type: ima-runtime
spec:
  # Use this as the default policy for all enrollments
  matchAll: true

  # Runtime policy with $COUNT IMA measurements
  # Auto-converted from allowlist format to Keylime JSON
  runtimePolicy: |
$(cat "$MEASUREMENTS_FILE" | sed 's/^/    /')

  # TPM policy with mask 0x0 means no PCR verification
  # Only IMA runtime measurements will be checked
  tpmPolicy: |
    {"mask": "0x0"}

  # Measured boot policy - empty JSON disables measured boot validation
  mbPolicy: |
    {}
EOF

echo ""
echo "✓ Created $OUTPUT_FILE"
echo ""
echo "To apply this policy:"
echo "  kubectl apply -f $OUTPUT_FILE"
echo ""
echo "Or using flightctl CLI:"
echo "  bin/flightctl apply -f $OUTPUT_FILE"
echo ""
echo "Note: The allowlist format will be automatically converted to Keylime JSON format by FlightCTL."
