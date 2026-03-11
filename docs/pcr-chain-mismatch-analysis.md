# PCR Chain Mismatch Analysis

## Problem Statement

Keylime reports: "IMA measurement list does not match TPM PCR"

The PCR 10 value calculated by replaying the IMA log doesn't match the PCR 10 value in the TPM quote.

## Investigation Summary

### What We Found

1. **Template Hash Validation Works Correctly** ✓
   - Keylime's IMA template data reconstruction is correct
   - The NULL byte after the colon is included (as per kernel source)
   - SHA1 hash of reconstructed template data matches kernel's template hash perfectly
   - Test confirmed: `SHA1(template_data) == expected_template_hash`

2. **PCR Bank Detection Works** ✓
   - Keylime correctly detects we're using SHA256 PCR bank
   - Log shows: "PCR(s) 0, 1, 2, 3, 4, 5, 6, 7 and 10 from bank 'sha256' found in TPM quote"

3. **IMA Log is Complete** ✓
   - We read from `/sys/kernel/security/ima/ascii_runtime_measurements`
   - This file contains the complete IMA log from boot (starts with boot_aggregate)
   - Size: ~300KB (2200+ entries)

4. **Timing is Correct** ✓
   - We generate TPM quote FIRST
   - Then read PCR values immediately after
   - Then read IMA log
   - Keylime handles the lag (IMA log can be ahead of PCR by a few entries)

### The Real Issue

The PCR replay fails because Keylime isn't finding a match while replaying the IMA log.

**Keylime's Replay Logic:**
```python
# Start with initial PCR value (all zeros for SHA256)
running_hash = TPMState.initial_pcr_value(config.IMA_PCR, hash_alg)

# Replay each IMA entry
for entry in ima_log:
    # Extend PCR: Hash(current_pcr || template_hash)
    running_hash = hash_alg.hash(running_hash + entry.pcr_template_hash)

    # Check if it matches the PCR from the quote
    if running_hash == pcrval_from_quote:
        found_pcr = True  # Success!
        break

# After replaying all entries
if not found_pcr:
    ERROR("IMA measurement list does not match TPM PCR")
```

**The issue:** `running_hash` never equals `pcrval_from_quote` during the entire replay.

## Root Cause Analysis

There are three possible causes:

### 1. Hash Algorithm Mismatch (Most Likely)

The `hash_alg` parameter passed to replay might default to SHA1 instead of SHA256.

**Evidence:**
```python
def process_measurement_list(
    ...
    hash_alg: algorithms.Hash = algorithms.Hash.SHA1,  # ← DEFAULT IS SHA1!
) -> Tuple[str, Failure]:
```

If Keylime is using SHA1 to extend the PCR during replay, but our TPM is using SHA256, the values will never match.

**What we need:**
- Verify that Keylime receives our `hash_alg: "sha256"` field from the request
- Ensure Keylime passes SHA256 (not SHA1) to `process_measurement_list()`
- Confirm the PCR extension uses SHA256: `SHA256(current_pcr || template_hash)`

### 2. Initial PCR Value Wrong

If the initial PCR value isn't all zeros, the replay starts from the wrong state.

**Should be:**
```python
initial_pcr = b'\x00' * 32  # 32 bytes of zeros for SHA256
```

### 3. PCR Data Format Issue

Our PCR data format might not be parsed correctly by Keylime, causing it to read the wrong PCR 10 value.

## Diagnostic Steps

To confirm the root cause, we need to log:

1. What PCR 10 value is in our quote
2. What PCR 10 value Keylime calculates after replaying the IMA log
3. What hash algorithm Keylime actually uses for PCR extension
4. What the initial PCR value is

## Next Steps

1. **Add Debug Logging** - Add logging to our agent to print PCR 10 value we read from TPM
2. **Verify Keylime Uses SHA256** - Check that Keylime's `process_measurement_list` receives `hash_alg=SHA256`
3. **Test PCR Replay Manually** - Replay the IMA log ourselves using SHA256 and verify it matches
4. **Consider Workarounds**:
   - Option A: Use allowlist-only validation (skip PCR chain check)
   - Option B: Fix Keylime to correctly use SHA256 for PCR extension
   - Option C: Provide a minimal IMA log for demo purposes

## Files Involved

- `/home/lily/go/src/flightctl/internal/tpm/session.go` - Quote generation and PCR reading
- `/home/lily/go/src/flightctl/internal/tpm/attestation.go` - IMA log reading
- `/home/lily/go/src/flightctl/internal/agent/device/lifecycle/manager.go` - Attestation flow
- `/keylime/keylime/ima/ima.py` - IMA log replay logic
- `/keylime/keylime/tpm/tpm_main.py` - TPM verification entry point
