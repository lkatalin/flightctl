#!/bin/bash
# Extract IMA measurements from a running VM
# Creates a timestamped file: measurements-YYYYMMDD-HHMMSS.txt

set -euo pipefail

VM_NAME="${1:-flightctl-device-default}"
VM_IP=$(virsh -c qemu:///system domifaddr "$VM_NAME" 2>/dev/null | grep -oE '192\.168\.[0-9]+\.[0-9]+' | head -1)

if [ -z "$VM_IP" ]; then
    echo "Error: Could not find IP address for VM: $VM_NAME"
    echo "Is the VM running?"
    virsh -c qemu:///system list --all | grep -i flightctl || true
    exit 1
fi

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_FILE="measurements-${TIMESTAMP}.txt"

echo "Extracting IMA measurements from VM: $VM_NAME ($VM_IP)"
echo "Output file: $OUTPUT_FILE"

# Find SSH key (prefer ed25519, fallback to rsa)
SSH_KEY=""
if [ -f ~/.ssh/id_ed25519 ]; then
    SSH_KEY=~/.ssh/id_ed25519
elif [ -f ~/.ssh/id_rsa ]; then
    SSH_KEY=~/.ssh/id_rsa
fi

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5"
if [ -n "$SSH_KEY" ]; then
    SSH_OPTS="$SSH_OPTS -i $SSH_KEY"
fi

# Extract measurements: hash filepath
# Try 'user' first (bootc default), fall back to 'core'
if ssh $SSH_OPTS user@"$VM_IP" 'sudo cat /sys/kernel/security/ima/ascii_runtime_measurements' 2>/dev/null \
    | awk '{print $4, $5}' > "$OUTPUT_FILE"; then
    : # Success with 'user'
elif ssh $SSH_OPTS core@"$VM_IP" 'sudo cat /sys/kernel/security/ima/ascii_runtime_measurements' 2>/dev/null \
    | awk '{print $4, $5}' > "$OUTPUT_FILE"; then
    : # Success with 'core'
else
    echo "Error: Could not SSH to VM as 'user' or 'core'"
    exit 1
fi

COUNT=$(wc -l < "$OUTPUT_FILE")
echo "✓ Extracted $COUNT measurements to $OUTPUT_FILE"
echo ""
echo "To create an attestation reference from this:"
echo "  ./create-attestation-ref-from-measurements.sh $OUTPUT_FILE"
