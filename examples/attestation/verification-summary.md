# Attestation Verification Summary

This document summarizes the attestation verification for enrollment request `oj02srr4f88rs3asmvnoum1a5nbdfhdo2k3j1imf0bmet625qkng`.

## Data Sent in the Attestation

### 1. TPM Quote (Cryptographically Signed Attestation)

- **Signed by**: LAK (Local Attestation Key)
- **PCR Selection**: `0xFF 0x00 0x00` = PCRs 0-7 only
  - PCR 0-7: Firmware and boot measurements
  - PCR 8-23: Excluded (includes PCR 10 which extends constantly)
- **Quote Digest**: Must match the separately-sent PCR values
- **Nonce**: Included to prevent replay attacks

### 2. PCR Values

- Raw values for PCRs 0-7
- Sent separately from the quote
- Used by Keylime to verify the quote digest matches

### 3. IMA Measurement List

- **Count**: 1,699 runtime file measurements
- **Format**: Each entry includes:
  - PCR index (10)
  - File hash (SHA-256)
  - Signature type (ima-sig)
  - File path
- **Examples**:
  - `/usr/bin/kmod`
  - `/usr/lib64/libc.so.6`
  - `/etc/ld.so.cache`
  - Kernel modules
  - System libraries
  - Configuration files

### 4. TPM Keys

- **TPM AK** (Attestation Key): Public key for verifying the quote signature
- **TPM EK** (Endorsement Key): TPM's unique identity key

### 5. Certificate Signing Request (CSR)

- Used to request a device identity certificate
- Generated with TPM-backed private key

---

## Policy Used for Verification

**AttestationReference**: `flightctl-baseline-policy`

### Runtime Policy (IMA)

```yaml
runtimePolicy: |
  {
    "meta": {
      "version": 1,
      "generator": 2,
      "timestamp": "2026-03-03T20:18:19Z"
    },
    "release": 0,
    "digests": {
      "/path/to/file": ["sha256-hash"],
      ...
    }
  }
```

- **Baseline measurements**: 95,382 file hashes from `measurements.txt`
- **Generated from**: Agent disk image during build
- **Validation**: All 1,699 runtime measurements matched against this baseline
- **Format**: JSON with file paths mapped to expected SHA-256 hashes

### TPM Policy

```yaml
tpmPolicy: |
  {"mask": "0x0"}
```

- **Mask 0x0**: NO PCR verification
- **Effect**: Keylime accepts any PCR values without validation
- **Rationale**: PCR values are included in quote for digest validation only

### Measured Boot Policy

```yaml
mbPolicy: |
  {}
```

- **Empty policy**: NO measured boot verification
- **Effect**: UEFI boot event logs are not validated
- **Rationale**: Focused on runtime integrity, not boot chain integrity

---

## What WAS Checked ✓

### Quote Validation

- ✓ Quote signature verified against TPM AK public key
- ✓ Quote digest matched the PCR values sent separately
- ✓ Quote included the correct nonce (prevents replay attacks)
- ✓ Quote was freshly generated (not replayed from earlier)

### IMA Runtime Integrity

- ✓ All 1,699 runtime file measurements matched the baseline policy
- ✓ No unexpected files were loaded into the system
- ✓ No files with incorrect/tampered hashes were found
- ✓ All kernel modules, libraries, and executables are known-good

### TPM Ownership Proof

- ✓ Device proved TPM ownership via activate-credential challenge
- ✓ Device successfully decrypted the challenge secret
- ✓ Confirms device has genuine TPM hardware

---

## What Was NOT Checked ✗

### PCR Values

- ✗ PCR 0-7 values were included but **NOT verified** (tpmPolicy mask = 0x0)
- ✗ Firmware measurements (PCR 0-2) were not validated
- ✗ BIOS/UEFI measurements (PCR 0-2) were not validated
- ✗ Bootloader measurements (PCR 4-5) were not validated
- ✗ Kernel and initrd measurements (PCR 8-9) were not validated

**Implication**: System could theoretically have booted from modified firmware, bootloader, or kernel, as long as runtime files match the baseline.

### Measured Boot

- ✗ UEFI event log was not checked (mbPolicy = {})
- ✗ Secure Boot state was not verified
- ✗ Boot component chain-of-trust was not validated
- ✗ Shim, GRUB, and kernel boot process not attested

### PCR 10 (IMA PCR)

- ✗ PCR 10 value was **excluded from quote** (by design)
- ✗ Only the IMA measurement list was validated, not the PCR value
- ✗ PCR 10 extends constantly at runtime, making it unsuitable for quotes

**Rationale**: IMA verification is done through the measurement list itself, not the PCR value. This provides more granular validation than a single PCR digest.

---

## Verification Result

**Status**: ✅ **Verification Succeeded**

```yaml
conditions:
  - type: AttestationVerified
    status: "True"
    reason: KeylimeVerificationSucceeded
    message: "Attestation verified by Keylime verifier using policy flightctl-baseline-policy"
    lastTransitionTime: "2026-03-03T20:46:54Z"

  - type: TPMVerified
    status: "True"
    reason: TPMChallengeSucceeded
    message: "TPM activate credential challenge completed successfully"
    lastTransitionTime: "2026-03-03T20:46:55Z"
```

### What This Confirms

The successful attestation proves:

1. **Genuine TPM**: Device has a real TPM 2.0 chip (not emulated/compromised)
2. **Runtime Integrity**: All running code matches the expected baseline
3. **No Tampering**: No unauthorized files or modified executables detected
4. **Known Configuration**: System is running the approved software stack

### What This Does NOT Confirm

The attestation does **not** guarantee:

1. **Boot Chain Integrity**: Firmware, bootloader, and kernel could be modified
2. **Secure Boot**: Secure Boot state and UEFI signature validation not checked
3. **Hardware Trust**: Only TPM authenticity is verified, not other hardware

---

## Technical Details

### Quote Format

The TPM quote contains:

- **TPMS_ATTEST** structure with:
  - Magic value: `0xFF544347` ("TCG" signature)
  - Type: TPM_ST_ATTEST_QUOTE
  - Qualified signer (LAK name)
  - Extra data (nonce)
  - Clock info (TPM clock and reset count)
  - Firmware version
  - **PCR selection**: `0x0B03FF000000` = SHA-256, PCRs 0-7
  - **PCR digest**: SHA-256 hash of concatenated PCR 0-7 values

### Validation Flow

```
1. Agent generates quote with PCRs 0-7
2. Agent reads PCR 0-7 values immediately after quote
3. Agent sends quote + PCRs + IMA list to FlightCTL
4. FlightCTL forwards to Keylime verifier
5. Keylime validates:
   a. Quote signature using TPM AK public key
   b. Quote digest matches sent PCR values
   c. IMA measurements match baseline policy
6. Keylime returns success/failure
7. FlightCTL updates enrollment condition
```

### Why PCRs After Quote?

Reading PCRs **after** the quote ensures they match what was quoted. This is critical because:

- PCR 10 (IMA) extends every time a file is measured
- Reading PCRs before the quote could cause mismatch if files load between read and quote
- Reading after ensures the values match the quote's internal PCR digest

---

## Comparison with Full Attestation

| Component | This Attestation | Full Attestation |
|-----------|------------------|------------------|
| Runtime files (IMA) | ✓ Verified | ✓ Verified |
| TPM ownership | ✓ Verified | ✓ Verified |
| Firmware (PCR 0-2) | ✗ Not verified | ✓ Verified |
| Bootloader (PCR 4-5) | ✗ Not verified | ✓ Verified |
| Kernel (PCR 8-9) | ✗ Not verified | ✓ Verified |
| Secure Boot | ✗ Not verified | ✓ Verified |
| UEFI event log | ✗ Not verified | ✓ Verified |

To enable full attestation, update the policy:

```yaml
spec:
  tpmPolicy: |
    {"mask": "0xFF03"}  # Verify PCRs 0-7,8-9

  mbPolicy: |
    {
      "pcrs": [0, 1, 2, 3, 4, 5, 6, 7],
      "evidence": ["bootloader", "kernel", "initrd"]
    }
```

---

## Related Files

- **Policy**: `examples/attestation/attestation-reference-from-measurements.yaml`
- **Baseline**: `measurements.txt` (95,382 measurements)
- **Generator**: `examples/attestation/create-attestation-ref-from-measurements.sh`
- **Helper**: `find-newest-enrollment.sh`
- **Rebuild**: `rebuild-agent-and-update-policy.sh`
