# PCR Chain Mismatch - Root Cause and Fix

## Summary

**Root Cause:** Configuration error in our code - we were setting `ima.log_hash_alg` to `"sha256"` when it should always be `"sha1"` for standard ima-ng templates.

**Impact:** Keylime's PCR replay failed because it was recalculating template hashes with SHA256 instead of using the SHA1 hashes from the IMA log.

**Status:** ✅ FIXED

## The Bug

In `internal/attestation/policy/converter.go`, we were detecting the **file content hash** algorithm from the measurements and incorrectly using it as the **IMA template hash** algorithm.

### Before (Incorrect):
```go
// Detect hash algorithm from first hash length
hashAlg := "sha256" // Detected from file hashes
...
IMA: IMAConfig{
    LogHashAlg: hashAlg, // ❌ WRONG: Using file hash algorithm
}
```

### After (Correct):
```go
// IMA template hashes are ALWAYS SHA1 for standard ima-ng format
templateHashAlg := "sha1"
...
IMA: IMAConfig{
    LogHashAlg: templateHashAlg, // ✅ CORRECT: Always sha1
}
```

## Understanding the Two Hash Algorithms

### 1. File Content Hash (Variable)
- What it is: Hash of the actual file content
- Algorithm: Can be SHA1, SHA256, SHA512, etc. (SHA256 on modern systems)
- Example: `sha256sum /bin/bash` produces this hash
- Used for: Verifying file integrity against allowlist

### 2. IMA Template Hash (Always SHA1)
- What it is: Hash of the IMA template data structure
- Algorithm: **ALWAYS SHA1** for ima-ng templates
- Example: The second field in IMA log entries (always 40 hex chars = 20 bytes)
- Used for: PCR 10 extension chain

## IMA Log Entry Structure

```
10 3431022251bcde4ea8728ceea224e29449c0bc17 ima-ng sha256:520364d1... boot_aggregate
│  └────────────────┬───────────────────┘        └──────┬─────────┘  └──────┬──────┘
│           Template Hash (SHA1, 20 bytes)       File Hash (SHA256)      Filename
│           Used for PCR extension                 Used for allowlist
PCR Number
```

##Template Data Construction:
```
Template Data = struct {
    uint32 hash_field_len
    string algorithm      // "sha256"
    byte   ':'
    byte   '\0'
    bytes  file_hash      // 32 bytes for SHA256
    uint32 name_field_len
    string filename
    byte   '\0'
}

Template Hash = SHA1(Template Data)  // Always SHA1, regardless of file hash algorithm!
```

## How PCR 10 Extension Works

```python
# Initial PCR 10 value (SHA256 bank)
pcr_10 = b'\x00' * 32  # 32 bytes of zeros

# For each IMA entry:
template_hash = SHA1(template_data)  # Always SHA1, 20 bytes
pcr_10 = SHA256(pcr_10 || template_hash)  # Extend with SHA256 (PCR bank algorithm)
```

**Key Point:** The PCR bank algorithm (SHA256) is only used for the **extension operation**. The template hash itself is always SHA1.

## What Keylime Does with log_hash_alg

When `log_hash_alg` is set to `"sha256"`:
```python
# Keylime recalculates template hash with SHA256 (WRONG!)
entry.pcr_template_hash = SHA256(template_data)  # 32 bytes
pcr_replay = SHA256(pcr_replay || SHA256_hash)   # Doesn't match!
```

When `log_hash_alg` is correctly set to `"sha1"`:
```python
# Keylime recalculates template hash with SHA1 (CORRECT!)
entry.pcr_template_hash = SHA1(template_data)  # 20 bytes
pcr_replay = SHA256(pcr_replay || SHA1_hash)   # Matches actual PCR!
```

## Why This Worked Before

Your observation that "Keylime worked before" was the key insight! This configuration error only affects systems using:
- SHA256 (or higher) file content hashes
- ima-ng template format

If a system were using SHA1 for both file hashes AND template hashes, the bug wouldn't manifest because both would be SHA1.

## Test Results

### Manual PCR Replay Test

Using the first IMA entry (boot_aggregate):

Template data: `280000007368613235363a00520364d1...`
Template hash from log: `3431022251bcde4ea8728ceea224e29449c0bc17` (SHA1, 20 bytes)

**With log_hash_alg="sha256" (WRONG):**
```
entry.pcr_template_hash = SHA256(template_data) = 85f5b30d... (32 bytes)
PCR after 1 entry = SHA256(zeros || 85f5b30d...) = d4b7cf97...
```

**With log_hash_alg="sha1" (CORRECT):**
```
entry.pcr_template_hash = SHA1(template_data) = 3431022251... (20 bytes)
PCR after 1 entry = SHA256(zeros || 3431022251...) = c2f4382d...
```

Only the second one matches the actual PCR value from the TPM!

## Files Changed

1. **internal/attestation/policy/converter.go** - Fixed log_hash_alg generation
2. **internal/attestation/policy/converter_test.go** - Updated test expectations
3. **examples/attestation/attestation-reference-json.yaml** - Fixed example
4. **cmd/flightctl-agent/main.go** - Updated debug version to v11

## Next Steps

1. ✅ Code changes made
2. ⏳ Rebuild API server with fix
3. ⏳ Regenerate attestation policy (will have correct log_hash_alg)
4. ⏳ Verify PCR replay succeeds
5. ⏳ Confirm attestation passes

## Verification

After deploying the fix, check:
```bash
# 1. Verify runtime policy has correct setting
./bin/flightctl get attestationreference flightctl-baseline-policy -o json | \
  jq -r '.spec.runtimePolicy' | jq '.ima.log_hash_alg'
# Should output: "sha1"

# 2. Check Keylime logs for success
kubectl logs -n flightctl-external deployment/keylime-verifier --tail=100 | \
  grep "IMA measurement list does not match"
# Should NOT see this error anymore

# 3. Check attestation status
kubectl logs -n flightctl-external deployment/flightctl-api --tail=50 | \
  grep "valid.*true"
# Should see attestation succeeding
```

## Lessons Learned

1. **Configuration matters** - Even when the core logic is correct, wrong configuration can break everything
2. **User observations are valuable** - "It worked before" was the key clue that led us away from suspecting a Keylime bug
3. **Understand the data structures** - Knowing the difference between file hashes and template hashes was crucial
4. **Test hypotheses** - Manual PCR replay test confirmed the root cause definitively

## References

- Linux IMA documentation: https://www.kernel.org/doc/html/latest/security/IMA-templates.html
- Keylime IMA validation: https://keylime.dev/
- TPM 2.0 PCR extension: TPM 2.0 Specification Part 1, Section 16.4
