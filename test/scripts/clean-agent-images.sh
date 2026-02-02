#!/usr/bin/env bash
# Clean all agent image artifacts and cached bundles to force a complete rebuild

set -e

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

cd "$ROOT_DIR"

echo "Cleaning agent VM and image artifacts..."

# Clean running VM if it exists
echo "  - Stopping any running agent VMs..."
sudo virsh destroy flightctl-device-default 2>/dev/null || true
sudo virsh destroy flightctl-e2e-worker-1 2>/dev/null || true
sudo rm -f /var/lib/libvirt/images/flightctl-device-default.qcow2 2>/dev/null || true
sudo rm -f /var/lib/libvirt/images/flightctl-e2e-worker-1.qcow2 2>/dev/null || true

# Clean disk images
echo "  - Removing disk images..."
rm -f bin/output/qcow2/disk.qcow2

# Clean sentinel files
echo "  - Removing sentinel files..."
rm -f bin/.e2e-agent-images*
rm -f bin/.e2e-agent-injected
rm -f bin/.e2e-agent-certs

# Clean image bundles (these are the cached pre-built images)
echo "  - Removing cached image bundles..."
rm -f bin/agent-artifacts/agent-images-bundle*.tar
rm -f bin/app-images-bundle.tar

# Clean agent config (will be regenerated)
echo "  - Removing agent config..."
rm -rf bin/agent/etc/flightctl/config.yaml

echo "✓ Agent image artifacts cleaned successfully"
echo ""
echo "Next steps:"
echo "  Run 'make attestation-demo' to rebuild everything from scratch"
echo "  Or run 'make attestation-agent-vm' to rebuild just the agent VM"
