#!/bin/bash
# Extract IMA measurements from a disk image by booting it in a VM
#
# This uses the same workflow that worked before:
# 1. Boot VM with attestation-agent-vm (sets up SSH access)
# 2. Extract measurements using the existing script
# 3. Clean up
#
# Usage: ./extract-measurements-from-disk.sh

set -euo pipefail

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
ROOT_DIR="$SCRIPT_DIR/../.."
VM_NAME="flightctl-device-default"

cd "$ROOT_DIR"

echo "=========================================="
echo "Extract Measurements from Disk"
echo "=========================================="
echo ""
echo "This will boot the VM to extract IMA measurements."
echo ""
echo "IMPORTANT: This uses existing agent config from bin/agent/ for SSH access."
echo "           The VM will try to connect to the server, but that's OK - we just"
echo "           need the VM booted long enough to extract measurements."
echo ""

# Check if agent config exists
if [ ! -f bin/agent/etc/flightctl/config.yaml ]; then
    echo "Error: No agent config found at bin/agent/etc/flightctl/config.yaml"
    echo ""
    echo "You need to generate agent config first. Options:"
    echo "  1. Start the server and run: make prepare-agent-config-attestation"
    echo "  2. Or run the full: make attestation-server-policy"
    echo ""
    exit 1
fi

echo "✓ Found existing agent config with SSH keys"
echo ""

# Clean up any existing VM first
make clean-agent-vm VMNAME="$VM_NAME" 2>/dev/null || true

# Boot VM - this uses existing agent config which includes SSH keys
echo "Booting VM (this takes 1-2 minutes)..."
# Use agent-vm directly with the existing config
make agent-vm VMNAME="$VM_NAME" VMWAIT=0

echo ""
echo "Waiting for VM to get IP address..."

# Wait for VM to get IP
MAX_WAIT=60
COUNTER=0
VM_IP=""
while [ $COUNTER -lt $MAX_WAIT ]; do
    VM_IP=$(sudo virsh -c qemu:///system domifaddr "$VM_NAME" 2>/dev/null | grep -oE '192\.168\.[0-9]+\.[0-9]+' | head -1 || true)
    if [ -n "$VM_IP" ]; then
        echo "✓ VM has IP: $VM_IP"
        break
    fi
    printf "."
    sleep 2
    COUNTER=$((COUNTER + 2))
done
echo ""

if [ -z "$VM_IP" ]; then
    echo "Error: VM did not get an IP address after ${MAX_WAIT}s"
    make -C "$ROOT_DIR" clean-agent-vm VMNAME="$VM_NAME" >/dev/null 2>&1 || true
    exit 1
fi

echo "Waiting for VM to respond to ping..."
MAX_WAIT=30
COUNTER=0
while [ $COUNTER -lt $MAX_WAIT ]; do
    if ping -c 1 -W 1 "$VM_IP" >/dev/null 2>&1; then
        echo "✓ VM is responding to ping"
        break
    fi
    printf "."
    sleep 1
    COUNTER=$((COUNTER + 1))
done
echo ""

if [ $COUNTER -ge $MAX_WAIT ]; then
    echo "Warning: VM not responding to ping, trying SSH anyway..."
fi

echo "Waiting for SSH to be available..."

# Find SSH key
SSH_KEY=""
if [ -f ~/.ssh/id_ed25519 ]; then
    SSH_KEY=~/.ssh/id_ed25519
elif [ -f ~/.ssh/id_rsa ]; then
    SSH_KEY=~/.ssh/id_rsa
fi

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=2"
if [ -n "$SSH_KEY" ]; then
    SSH_OPTS="$SSH_OPTS -i $SSH_KEY"
fi

# Wait for SSH to be ready (try both users)
MAX_WAIT=60
SSH_READY=false
for user in user core; do
    COUNTER=0
    while [ $COUNTER -lt $MAX_WAIT ]; do
        if ssh $SSH_OPTS $user@"$VM_IP" 'echo ready' >/dev/null 2>&1; then
            echo "✓ SSH is ready (user: $user)"
            SSH_READY=true
            break 2
        fi
        printf "."
        sleep 2
        COUNTER=$((COUNTER + 2))
    done
    echo ""
done

if [ "$SSH_READY" != "true" ]; then
    echo "Error: SSH did not become available after trying both 'user' and 'core' accounts"
    make -C "$ROOT_DIR" clean-agent-vm VMNAME="$VM_NAME" >/dev/null 2>&1 || true
    exit 1
fi

echo ""
echo "Extracting measurements..."
echo ""

cd "$SCRIPT_DIR"
if ! ./extract-measurements-from-vm.sh "$VM_NAME"; then
    echo "Error: Failed to extract measurements"
    make -C "$ROOT_DIR" clean-agent-vm VMNAME="$VM_NAME" >/dev/null 2>&1 || true
    exit 1
fi

# Get the latest measurements file
LATEST=$(ls -t measurements-*.txt 2>/dev/null | head -1)

if [ -z "$LATEST" ] || [ ! -f "$LATEST" ]; then
    echo "Error: No measurements file was created"
    make -C "$ROOT_DIR" clean-agent-vm VMNAME="$VM_NAME" >/dev/null 2>&1 || true
    exit 1
fi

COUNT=$(wc -l < "$LATEST")
FULL_PATH="$SCRIPT_DIR/$LATEST"

echo ""
echo "=========================================="
echo "Measurements Extracted!"
echo "=========================================="
echo ""
echo "✓ Extracted $COUNT measurements"
echo ""
echo "To compare with baseline (95,382 measurements from March 11):"
echo "  diff examples/attestation/measurements-20260311-132737.txt examples/attestation/$LATEST"
echo ""
echo "To create an attestation reference:"
echo "  cd examples/attestation && ./create-attestation-ref-from-measurements.sh $LATEST"
echo ""
echo "VM will be cleaned up when you exit..."
read -p "Press Enter to clean up VM and exit..."
make -C "$ROOT_DIR" clean-agent-vm VMNAME="$VM_NAME"
echo ""
echo "Measurements saved to: $FULL_PATH"
