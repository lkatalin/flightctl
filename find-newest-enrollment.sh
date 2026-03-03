#!/bin/bash
# Find the newest enrollment request

echo "Finding newest enrollment request..."

NEWEST_TIME=""
NEWEST_NAME=""

# Get all enrollment request names
for name in $(bin/flightctl get enrollmentrequest -o yaml | grep "^    name:" | grep -v "hostname" | awk '{print $2}'); do
  # Get creation timestamp for this enrollment
  timestamp=$(bin/flightctl get enrollmentrequest "$name" -o yaml | grep "creationTimestamp:" | head -1 | awk '{print $2}' | tr -d '"')

  # Compare timestamps (lexicographically works for ISO8601)
  if [[ "$timestamp" > "$NEWEST_TIME" ]] || [[ -z "$NEWEST_TIME" ]]; then
    NEWEST_TIME="$timestamp"
    NEWEST_NAME="$name"
  fi
done

echo ""
echo "Newest enrollment request:"
echo "  Name:      $NEWEST_NAME"
echo "  Created:   $NEWEST_TIME"
echo ""
echo "To check attestation status, run:"
echo "  bin/flightctl get enrollmentrequest $NEWEST_NAME -o yaml | grep -B3 -A3 'type: AttestationVerified'"
