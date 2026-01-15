# SWTPM Test CA Certificates

This directory is intentionally kept empty. Test swtpm CA certificates are automatically created during deployment.

## Automatic Test CA Creation

When you run `TPM=enabled make deploy`, a **single test swtpm CA** is automatically created for the entire test environment. This ensures both the server and agent-VMs use the same CA, eliminating certificate mismatch issues.

**⚠️ WARNING: This is for TESTING ONLY. Never use in production.**

### How It Works

1. **Server deployment** (`TPM=enabled make deploy`):
   - Creates a new test swtpm CA in `bin/swtpm-ca/`
   - Generates root CA (`swtpm-localca-rootca`) and intermediate CA (`swtpm-localca`)
   - Deploys these CA certificates to the server
   - Server trusts EK certificates signed by this test CA

2. **Agent-VM creation** (`TPM=enabled make agent-vm`):
   - Configures the VM's swtpm emulator to use the same test CA from `bin/swtpm-ca/`
   - VM's TPM generates EK certificate signed by the test CA
   - Server can validate the EK certificate because it trusts the test CA

### Files Created

When `TPM=enabled make deploy` runs, it creates `bin/swtpm-ca/` with:

- `swtpm-localca-rootca-cert.pem` - Root CA certificate (CN=swtpm-localca-rootca)
- `swtpm-localca-rootca-privkey.pem` - Root CA private key
- `issuercert.pem` - Intermediate CA certificate (CN=swtpm-localca)
- `signkey.pem` - Intermediate CA private key (used to sign EK certificates)
- `certserial` - Certificate serial number tracker

**You don't need to manually create or copy any certificates.**

## Certificate Chain

The test CA creates this certificate chain:

1. **Root CA**: `swtpm-localca-rootca` (CN=swtpm-localca-rootca)
   - Self-signed, valid for 10 years
2. **Intermediate CA**: `swtpm-localca` (CN=swtpm-localca)
   - Signed by root CA, used to sign EK certificates
3. **EK Certificate**: Agent-VM's TPM endorsement key certificate
   - Signed by intermediate CA during VM boot

Both the root and intermediate CA certificates are deployed to the server, allowing it to validate the complete chain.

## Usage

### Normal Workflow

```bash
# 1. Deploy server with TPM support (creates test CA)
TPM=enabled make deploy

# 2. Create agent-VM (uses the test CA)
TPM=enabled make agent-vm

# Agent enrolls successfully because server trusts the test CA
```

### Troubleshooting

If TPM enrollment fails with "certificate signed by unknown authority":

1. **Ensure consistent TPM=enabled usage**:
   ```bash
   # Both commands must use TPM=enabled
   TPM=enabled make deploy
   TPM=enabled make agent-vm
   ```

2. **Check test CA was created**:
   ```bash
   ls -la bin/swtpm-ca/
   # Should show signkey.pem, issuercert.pem, etc.
   ```

3. **Verify CA is deployed**:
   ```bash
   kubectl get configmap -n flightctl-external tpm-ca-certs \
     -o jsonpath='{.data}' | grep swtpm
   ```

4. **Clean and redeploy if needed**:
   ```bash
   make clean-swtpm-certs
   TPM=enabled make deploy
   ```

## Cleanup

```bash
# Remove test CA and VM configuration
make clean-swtpm-certs

# Remove agent-VM
make clean-agent-vm
```

## Production Use

**⚠️ WARNING:** This test CA is for development and testing ONLY.

For production:
- Use real TPM hardware (not emulated swtpm)
- Real TPMs have EK certificates signed by manufacturer CAs (Infineon, Nuvoton, STMicroelectronics, etc.)
- Deploy actual manufacturer CA certificates to the server
- Never use self-generated test CAs

## Technical Details

The test CA is created by `test/scripts/create-test-swtpm-ca.sh`, which:
- Generates a 2048-bit RSA root CA
- Generates a 2048-bit RSA intermediate CA
- Configures proper X.509 extensions for CA usage
- Creates the certificate chain structure expected by swtpm

When `TPM=enabled make agent-vm` runs:
- Creates temporary swtpm configuration pointing to `bin/swtpm-ca/`
- Passes this config to virt-install via `SWTPM_LOCALCA_CONF` environment variable
- swtpm reads the config and uses the test CA to sign the EK certificate
- VM boots with EK certificate that the server can validate
