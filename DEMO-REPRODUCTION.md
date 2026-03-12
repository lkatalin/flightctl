  # Exact Demo Reproduction

  This tag represents a working demo with ZERO attestation errors.

  ## Binary Artifacts
  - Disk image: bin/output/qcow2/disk.qcow2 (1.6GB)
    - SHA256: 8104c01c4ae932803894e0988de93cfb373b186f0ef6b6013ac35af430205f1d
    - Built: 2026-03-11 13:27:37

  - Agent bundle: bin/agent-artifacts/agent-images-bundle-cs9-bootc.tar (2.6GB)
    - SHA256: 1062feb4689d96f09cbc551bff49df1646938b21256caa5b693501dd2d1d8423

  ## To Rebuild
  make AGENT_OS_ID=cs9-bootc e2e-agent-images

  This will produce identical artifacts if built from the same source commit.

