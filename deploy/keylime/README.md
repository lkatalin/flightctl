# Keylime Verifier - Build from Latest Master

This directory contains files to build a Keylime verifier image from the latest upstream/master branch.

## Why Build from Master?

The pre-built `quay.io/keylime/keylime_verifier:latest` image may not include the most recent changes from the Keylime repository. To ensure you have the latest features and fixes (including v2.4 API updates), build from the upstream master branch.

## Quick Start

### Option 1: Build Locally for Kind/Minikube

```bash
# Build and load into Kind cluster
docker build -t keylime-verifier:master deploy/keylime/
kind load docker-image keylime-verifier:master

# Update values.yaml:
#   keylime:
#     image:
#       repository: keylime-verifier
#       tag: master
#       pullPolicy: Never
```

### Option 2: Build and Push to Registry

```bash
# Build and push to your registry
./deploy/keylime/build-and-push.sh quay.io/myorg/keylime-verifier:latest

# Update values.yaml with the output from the script
```

### Option 3: Use Pre-built Image (May Not Have Latest Changes)

```bash
# Use the official image (default in values.yaml)
# This may not include changes from the past week
#   keylime:
#     image:
#       repository: quay.io/keylime/keylime_verifier
#       tag: latest
```

## What's Included

The Dockerfile:
- Uses Fedora base image (Keylime's recommended distro)
- Clones the latest Keylime from https://github.com/keylime/keylime master branch
- Installs all dependencies
- Runs the verifier with latest code including v2.4 API support

## Updating to Latest Master

To rebuild with the newest changes:

```bash
# Rebuild (this will git clone the latest master)
./deploy/keylime/build-and-push.sh quay.io/myorg/keylime-verifier:$(date +%Y%m%d)

# Or for local development
docker build --no-cache -t keylime-verifier:master deploy/keylime/
kind load docker-image keylime-verifier:master
```

## Verifying the Build

Check that you have the v2.4 API:

```bash
# After deploying, check the API version
kubectl exec -n flightctl-external deployment/keylime-verifier -- \
  curl -sk https://localhost:8881/version

# Should show API version 2.4 is supported
```
