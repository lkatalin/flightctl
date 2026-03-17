# Attestation Troubleshooting Guide

## Helm Upgrade Conflicts

### Symptom

When running `make attestation-server-policy`:
```
Error: UPGRADE FAILED: conflict occurred while applying object flightctl-external/flightctl-api-config /v1,
Kind=ConfigMap: Apply failed with 1 conflict: conflict with "kubectl-patch" using v1: .data.config.yaml
```

### Root Cause

Helm is trying to upgrade an existing deployment, but resources were manually modified or a previous deployment exists.

### Solution

Clean up the old deployment first:

```bash
# Clean the cluster
make clean-cluster

# Then deploy fresh
make attestation-server-policy
```

If that doesn't work, delete and recreate the cluster:

```bash
kind delete cluster
kind create cluster
make attestation-server-policy
```

**Note**: This will delete all data in the cluster. Make sure you don't have important data before running.

## Certificate Mismatch Errors

### Symptom

VM agent logs show:
```
failed to create enrollment request: Post "https://agent-api...": tls: failed to verify certificate:
x509: certificate signed by unknown authority
```

### Root Cause

The agent config in `bin/agent/etc/flightctl/config.yaml` contains an old CA certificate that doesn't match the current server's CA certificate.

This happens when:
1. Server is deployed/redeployed (generates new certs)
2. Agent config is not regenerated
3. VM boots with old config

### Solution

The fix depends on your situation:

**If you already have a server running and just need to update agent config:**

```bash
make prepare-agent-config-attestation
make clean-agent-vm
make attestation-agent-vm
```

**If you're deploying from scratch (recommended):**

```bash
# This automatically generates agent config after deploying the server
make attestation-server-policy
make attestation-agent-vm
```

### Complete Workflow (Correct Order)

For testing attestation from scratch:

```bash
# 1. Deploy server with attestation (auto-generates agent config)
make attestation-server-policy

# 2. Boot VM with attestation (auto-regenerates config if needed)
make attestation-agent-vm

# 3. Check enrollment status
bin/flightctl get enrollmentrequests
```

**Note**: Both `attestation-server-policy` and `attestation-agent-vm` automatically handle agent config generation, so you typically don't need to run `make prepare-agent-config-attestation` manually.

### Why Does This Happen?

When the server is deployed:
- New CA certificates are generated in `bin/e2e-certs/`
- These are used by the server for TLS

The agent needs:
- The CA certificate to verify the server
- Client certificates to authenticate

If the agent config is not regenerated after server deployment, it will have the old CA and cannot trust the server's new certificate.

### Prevention

**Always use the correct make targets:**

1. **Deploy server**: `make attestation-server-policy` (auto-generates agent config)
2. **Boot VM**: `make attestation-agent-vm` (auto-regenerates config if needed)

**Don't use**: `make agent-vm` for attestation testing - it uses cached config without checking if it matches the server.

**Don't manually run**: `make prepare-agent-config-attestation` unless troubleshooting - it's already included in the targets above.

### For Teammates

If a teammate encounters certificate errors:

**Option 1 - Quick fix (if server is already running):**
```bash
make prepare-agent-config-attestation
make clean-agent-vm
make attestation-agent-vm
```

**Option 2 - Clean slate (recommended):**
```bash
make clean-cluster
make attestation-server-policy  # Auto-generates agent config
make attestation-agent-vm       # Auto-regenerates if needed
```

### Checking If Config Matches Server

Compare timestamps:

```bash
# When was the server deployed?
ls -l bin/e2e-certs/ca.pem

# When was agent config generated?
ls -l bin/agent/etc/flightctl/config.yaml
```

If `ca.pem` is newer than `config.yaml`, regenerate the agent config.

### Advanced: Extracting Measurements

When using `examples/attestation/extract-measurements-from-disk.sh`:

This script boots a VM just to extract measurements (not for enrollment). It uses existing agent config for SSH access, which is fine for measurement extraction but may cause confusion if you later try to enroll that VM.

**Best practice**: After extracting measurements, always regenerate config before attestation testing:

```bash
# Extract measurements
examples/attestation/extract-measurements-from-disk.sh

# Before attestation testing, ensure fresh config
make prepare-agent-config-attestation
make clean-agent-vm
make attestation-agent-vm
```

## No Enrollment Requests Appearing

### Symptoms

- VM is running
- Agent is running (check `make agent-vm-console`)
- But `bin/flightctl get enrollmentrequests` shows nothing

### Possible Causes

1. **Certificate mismatch** (see above)
2. **Worker/Periodic pods not running**
3. **Network connectivity issues**
4. **VM not getting IP address**

### Check Worker and Periodic Pods

These are required to process enrollments:

```bash
kubectl get pods -n flightctl-internal | grep -E "worker|periodic"
```

Should show:
```
flightctl-periodic-xxx   1/1   Running
flightctl-worker-xxx     1/1   Running
```

If missing, redeploy the server:
```bash
make clean-cluster
make attestation-server-policy
```

### Check VM Network

```bash
# Get VM IP
sudo virsh domifaddr flightctl-device-default

# Should show an IP like: 192.168.122.x
```

If no IP, check libvirt network:
```bash
sudo virsh net-list
sudo virsh net-dhcp-leases default
```

### Check Agent Logs

From the VM console (`make agent-vm-console`):

```bash
# Check if agent is running
sudo systemctl status flightctl-agent

# View agent logs
sudo journalctl -u flightctl-agent -f
```

Look for:
- Certificate errors (see certificate section above)
- Network errors (DNS resolution, connection refused)
- TPM errors (if attestation is enabled)

## Enrollment Succeeds But No Attestation Verification

### Check Keylime Verifier

```bash
kubectl get pods -n flightctl-external | grep keylime
kubectl logs -n flightctl-external deployment/keylime-verifier
```

### Check API Server Logs

```bash
kubectl logs -n flightctl-external deployment/flightctl-api | grep -i attestation
```

### Check Attestation Reference

```bash
# List attestation references
bin/flightctl get attestationreferences

# Check if policy is applied
bin/flightctl get attestationreference default-ima-policy -o yaml
```
