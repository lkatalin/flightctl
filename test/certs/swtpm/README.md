# SWTPM Test CA Certificates

This directory is intentionally kept empty. swtpm CA certificates are auto-discovered during deployment.

## Automatic Certificate Discovery

When you run `TPM=enabled make deploy`, the deployment script automatically discovers
and copies the swtpm CA certificates from your system:

1. **First tries**: `/var/lib/swtpm-localca/` (system-wide swtpm, used by libvirt/QEMU)
   - **Note**: This requires sudo access to read the certificates
   - You may be prompted for your password during deployment
2. **Falls back to**: `~/.config/var/lib/swtpm-localca/` (user-specific swtpm)
   - No sudo required

This ensures the deployed CA certificates always match the actual swtpm installation
that will sign EK certificates for your agent VMs.

**You don't need to manually copy any certificates to this directory.**

## Certificate Chain

When using emulated TPM (swtpm) for development and testing, the TPM endorsement key
certificates are signed by a certificate chain:

1. **Root CA**: `swtpm-localca-rootca` (CN=swtpm-localca-rootca)
2. **Intermediate CA**: `swtpm-localca` (CN=swtpm-localca) - signs EK certificates
3. **EK Certificate**: The TPM's endorsement key certificate

Both the root and intermediate CA certificates must be trusted by the Flight Control
API server to validate these EK certificates during device enrollment.

The deployment script automatically copies both certificates from your swtpm installation.

## Troubleshooting

If TPM enrollment fails with "certificate signed by unknown authority":

1. Check that swtpm certificates exist on your system:
   ```bash
   ls -la /var/lib/swtpm-localca/
   # or
   ls -la ~/.config/var/lib/swtpm-localca/
   ```

2. Verify the deployment found the certificates:
   ```bash
   TPM=enabled make deploy-tpm-certs
   # Look for "Found system-wide swtpm-localca" or "Found user swtpm-localca"
   ```

3. If you get a "WARNING: No swtpm-localca certificates found" message, your system
   may not have swtpm configured yet. The certificates are created when swtpm first runs.

## Production Use

**⚠️ WARNING:** swtpm is for testing only. DO NOT use in production.

For production environments, use real TPM hardware which will have endorsement key
certificates signed by actual TPM manufacturer CAs (Infineon, Nuvoton, STMicroelectronics, etc.).

## How It's Used

When `TPM=enabled` is set during deployment:
1. The deployment script finds your system's swtpm CA certificates
2. Copies them to `bin/tpm-cas/` along with manufacturer CAs
3. Deploys all CAs to the Kubernetes cluster
4. The API server can then validate EK certificates from agent VMs with emulated TPMs
