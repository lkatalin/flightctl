#!/usr/bin/env bash
# Check if attestation is enabled in a qcow2 disk image (works for OSTree/bootc images)
set -euo pipefail

QCOW="${1:-bin/output/qcow2/disk.qcow2}"

[[ -f "$QCOW" ]] || { echo "ERROR: Disk image not found: $QCOW" >&2; exit 1; }

check_path() {
    local dev="$1"
    local path="$2"
    if sudo virt-cat -a "$QCOW" -m "$dev" "$path" 2>/dev/null | grep -q "attestation-enabled: true"; then
        echo "✓ Attestation enabled (found at $path on $dev)"
        return 0
    fi
    return 1
}

# Find the root filesystem partition (usually /dev/sda4 for bootc images)
# Try common partition names
for dev in /dev/sda4 /dev/vda4 /dev/sda3 /dev/vda3; do
    # Try /etc first (non-OSTree systems or after OSTree creates the symlink)
    if check_path "$dev" "/etc/flightctl/config.yaml"; then
        exit 0
    fi

    # For OSTree/bootc systems, look for INJECTION_OK marker file
    for etc_path in $(sudo virt-ls -a "$QCOW" -m "$dev" -R /ostree/deploy 2>/dev/null | grep '/deploy/.*\.0/etc$' | head -5 || true); do
        injection_marker="/ostree/deploy/$etc_path/flightctl/INJECTION_OK"
        if sudo virt-cat -a "$QCOW" -m "$dev" "$injection_marker" 2>/dev/null >/dev/null; then
            # Found injection marker, now check config
            config_path="/ostree/deploy/$etc_path/flightctl/config.yaml"
            if check_path "$dev" "$config_path"; then
                exit 0
            else
                echo "WARNING: Found injection marker at $injection_marker but attestation not enabled in $config_path" >&2
                exit 1
            fi
        fi
    done
done

echo "Attestation not enabled (no config found in /etc or /ostree/deploy)" >&2
exit 1
