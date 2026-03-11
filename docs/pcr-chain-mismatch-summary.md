# PCR Chain Mismatch Investigation - Summary

## Executive Summary

Keylime is correctly detecting our SHA256 PCR bank and parsing our attestation data, but the PCR 10 replay fails because the calculated PCR doesn't match the quoted PCR. The most likely cause is that Keylime defaults to using SHA1 for PCR extensions during IMA log replay, even though we're using a SHA256 PCR bank.

## Investigation Findings

### ✓ What's Working Correctly

1. **IMA Template Hash Validation** - CONFIRMED WORKING
   - Keylime's template data reconstruction is 100% correct
   - Format: `len + 'sha256' + ':' + '\0' + hash + len + filename + '\0'`
   - My test: `SHA1(reconstructed_data) == kernel_template_hash` ✓
   - The NULL byte after colon is correct (as per Linux kernel source)

2. **PCR Bank Detection** - CONFIRMED WORKING
   - Keylime logs: "PCR(s) 0, 1, 2, 3, 4, 5, 6, 7 and 10 from bank 'sha256' found in TPM quote"
   - Keylime correctly detects we're using SHA256

3. **IMA Log Completeness** - CONFIRMED WORKING
   - We read from `/sys/kernel/security/ima/ascii_runtime_measurements`
   - This contains the complete log from boot (starts with boot_aggregate)
   - Size: ~300KB with 2200+ entries

### ❌ What's Failing

**Error:** "IMA measurement list does not match TPM PCR"

Keylime replays the entire IMA log but never finds a point where the running PCR hash matches the PCR 10 value from our quote.

## Root Cause: Hash Algorithm Mismatch

### The Problem

Looking at Keylime's code in `/keylime/keylime/ima/ima.py`:

```python
def process_measurement_list(
    agentAttestState: Optional[AgentAttestState],
    lines: List[str],
    runtime_policy: Optional[RuntimePolicyType] = None,
    pcrval: Optional[str] = None,
    ima_keyrings: Optional[ImaKeyrings] = None,
    boot_aggregates: Optional[Dict[str, List[str]]] = None,
    hash_alg: algorithms.Hash = algorithms.Hash.SHA1,  # ← DEFAULT IS SHA1!
) -> Tuple[str, Failure]:
```

**The default `hash_alg` is SHA1**, not SHA256!

### How Keylime Replays IMA Log

```python
# Start with initial PCR value (all zeros)
running_hash = TPMState.initial_pcr_value(config.IMA_PCR, hash_alg)

# For each IMA entry
for entry in ima_log:
    # Calculate template hash (always SHA1)
    template_hash = SHA1(template_data)  # 20 bytes

    # Extend PCR using hash_alg
    running_hash = hash_alg.hash(running_hash + template_hash)

    # Check if it matches quote
    if running_hash == pcrval_from_quote:
        success!
```

### The Issue

If `hash_alg` defaults to SHA1:
- Keylime extends with SHA1: `SHA1(current_pcr || template_hash)`
- But our TPM extends with SHA256: `SHA256(current_pcr || template_hash)`
- These will **never** match!

**Key insight:** The PCR extension algorithm must match what the TPM hardware uses. We're using the SHA256 PCR bank, so extensions must use SHA256, not SHA1.

### Why We're Using SHA256

Our agent code in `internal/tpm/client.go`:

```go
func (c *client) GetHashAlgorithm() string {
    // Currently hardcoded to SHA256
    return "sha256"
}
```

We send `hash_alg: "sha256"` to Keylime in the request, but Keylime may not be using it for PCR replay.

## Verification Needed

To confirm this is the issue, we need to check:

1. **What hash_alg is Keylime actually using?**
   - Add debug logging in Keylime's `process_measurement_list` to print the `hash_alg` parameter
   - Verify it's SHA256, not SHA1

2. **Manual PCR replay test**
   - Get the IMA log
   - Get the PCR 10 value from the quote
   - Manually replay using SHA256 and verify it matches

## Solution Options

### Option 1: Fix Keylime to Use SHA256 (Recommended)

Ensure Keylime receives and uses our `hash_alg: "sha256"` for PCR extension during IMA replay.

**Required changes:**
- Verify the `/v2.5/verify/evidence` endpoint receives `hash_alg` from request data
- Ensure it's passed through to `process_measurement_list(hash_alg=SHA256)`
- Add logging to confirm

### Option 2: Use Allowlist-Only Validation (Workaround)

We already have this working in branch `keylime-demo-allowlist-only`.

```bash
git checkout keylime-demo-allowlist-only
make attestation-demo-rebuild-agent
```

This skips PCR chain validation and only validates file hashes against the allowlist.

**Pros:**
- Works now
- Sufficient for demo purposes
- Still validates file integrity

**Cons:**
- Doesn't verify the tamper-proof PCR 10 chain
- Less rigorous than full attestation

### Option 3: Test with Minimal IMA Log

Create a test with just 3-5 IMA entries and manually verify the PCR replay works.

## Next Steps

1. **Immediate:** Decide between full PCR validation (Option 1) vs allowlist-only (Option 2) for the demo

2. **If pursuing Option 1:**
   - Add debug logging to Keylime to print `hash_alg` parameter
   - Verify it receives SHA256
   - Test manual PCR replay with SHA256
   - Fix Keylime if needed

3. **If choosing Option 2:**
   - Switch to `keylime-demo-allowlist-only` branch
   - Verify attestation works
   - Document limitations for demo

## Code Changes Made

Added debug logging in `internal/tpm/session.go` to print PCR values after quote generation:

```go
// DEBUG: Print PCR values, especially PCR 10 for IMA
s.log.Infof("DEBUG: PCR values read from TPM:")
for i, digest := range allPCRValues.Digests {
    if len(digest.Buffer) > 0 {
        s.log.Infof("DEBUG:   PCR[%d]: %x", i, digest.Buffer)
    }
}
```

This will help verify what PCR 10 value we're sending vs what Keylime expects.

## Files Analyzed

- `/home/lily/go/src/flightctl/internal/tpm/session.go` - Quote generation
- `/home/lily/go/src/flightctl/internal/tpm/attestation.go` - IMA log reading
- `/home/lily/go/src/flightctl/internal/agent/device/lifecycle/manager.go` - Attestation flow
- `/keylime/keylime/ima/ima.py` - IMA log replay (hash_alg default = SHA1)
- `/keylime/keylime/ima/ast.py` - Template data reconstruction (working correctly)
- `/keylime/keylime/tpm/tpm_main.py` - TPM verification entry point

## Conclusion

The PCR chain mismatch is **not** due to incorrect template data reconstruction (that works perfectly). It's likely due to Keylime using **SHA1** for PCR extensions during replay when it should use **SHA256** to match our TPM's PCR bank.

The allowlist-only workaround (Option 2) is ready to use if needed for the demo.
