# Understanding IMA File Hashes vs Template Hashes

## IMA Log Structure

When a file is measured by IMA, it creates a log entry that looks like this:

```
10 41022251bcde4ea8728ceea224e29449c0bc17 ima-ng sha256:5203641d151cfc15c5b0d9bb5476c2b487f77133db4b9b83f4a096666007d977f /usr/bin/kmod
```

Breaking this down:

| Field | Value | What it is |
|-------|-------|------------|
| PCR Index | `10` | Which PCR this extends |
| **Template Hash** | `41022251bcde...` | SHA1 hash of the entire entry |
| Template Name | `ima-ng` | IMA template format |
| File Hash Type | `sha256:` | Algorithm used for file hash |
| **File Hash** | `5203641d151c...` | SHA256 hash of `/usr/bin/kmod` file content |
| File Path | `/usr/bin/kmod` | The measured file |

## The Two Different Hashes

### 1. File Hash (SHA256 in our case)
```
sha256:5203641d151cfc15c5b0d9bb5476c2b487f77133db4b9b83f4a096666007d977f
```
- **What:** Hash of the actual file `/usr/bin/kmod` on disk
- **Algorithm:** SHA256 (configurable: `ima_hash=sha256` boot param)
- **Purpose:** Verifies the file hasn't been tampered with
- **Used in:** Runtime policy allowlists

### 2. Template Hash (SHA1 in our case)
```
41022251bcde4ea8728ceea224e29449c0bc17
```
- **What:** Hash of the **template data structure** for this log entry
- **Template data:** Binary blob containing:
  - Template format (`ima-ng`)
  - File hash (`sha256:5203...`)
  - File path (`/usr/bin/kmod`)
  - Other metadata
- **Algorithm:** SHA1 by default (requires kernel recompile or boot param to change)
- **Purpose:** Creates a tamper-proof chain in PCR 10
- **Used in:** Extending PCR 10 to build the measurement chain

## How PCR 10 Extension Works

```
1. PCR 10 starts at:    0000000000000000000000000000000000000000
2. Extend with entry 1: PCR10 = SHA1(PCR10 || template_hash_1)
3. Extend with entry 2: PCR10 = SHA1(PCR10 || template_hash_2)
4. Extend with entry 3: PCR10 = SHA1(PCR10 || template_hash_3)
...
N. Final PCR 10 value:  4dfc8170b6cb8ef9fd684fd3a9b547bc7eadb5c9
```

Each **template hash** is extended into PCR 10 to create a chain.

## The Validation Problem

When Keylime validates IMA with the full measurement list:

1. **Keylime receives:** ASCII IMA log with both hashes
2. **Keylime reconstructs:** The binary template data from the ASCII entry
3. **Keylime hashes it:** Using SHA1 to get the template hash
4. **Keylime expects:** This to match the template hash in the log
5. **But it fails because:**
   - The kernel might have used SHA256 for template hashing (newer kernels)
   - Or there's some format difference in how we're reconstructing it
   - The template hash in column 2 doesn't match Keylime's calculation

## Why Allowlist Format Bypasses This

With allowlist format:
```
5203641d151cfc15c5b0d9bb5476c2b487f77133db4b9b83f4a096666007d977f /usr/bin/kmod
```

Keylime only checks:
- ✓ Does `/usr/bin/kmod` appear in the IMA log?
- ✓ Does its **file hash** match `5203641d...`?
- ✗ **Skips** template hash validation entirely!

This is why allowlist is more forgiving and perfect for demos.

## Visual Diagram

```
IMA Log Entry:
┌─────────────────────────────────────────────────────────────────┐
│ 10  41022251bcde...  ima-ng  sha256:5203641d...  /usr/bin/kmod │
└─────────────────────────────────────────────────────────────────┘
     │                           │
     │                           └─ File Hash (SHA256)
     │                              "Hash of the file content"
     │                              Used in: Allowlists
     │
     └─ Template Hash (SHA1)
        "Hash of this entire entry's data structure"
        Used to: Extend PCR 10
```

## Key Insight

- **File hash** = "Is this the correct file?"
- **Template hash** = "Is this log entry tamper-proof?"

## Solutions

### Quick Win: Allowlist Mode (Current v9)
- Use allowlist-format runtime policy
- Don't send full IMA measurement list
- Keylime validates file hashes only
- Bypasses template hash validation

### Full Solution: Fix Template Hash Validation
- Understand exact binary format Keylime expects
- Match kernel's template data reconstruction
- Support both SHA1 and SHA256 template hashes
- Validate entire measurement chain

## References

- [IMA Documentation](https://www.kernel.org/doc/html/latest/security/IMA-templates.html)
- [Keylime IMA Validation](https://keylime.readthedocs.io/en/latest/user_guide/runtime_ima.html)
- TPM 2.0 spec for PCR extend operations
