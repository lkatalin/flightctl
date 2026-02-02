#!/bin/bash
# Build Keylime verifier from latest upstream/master and push to registry
# Usage: ./build-and-push.sh [IMAGE_NAME]
#
# Example:
#   ./build-and-push.sh quay.io/myorg/keylime-verifier:latest
#   ./build-and-push.sh localhost:5000/keylime-verifier:latest

set -e

IMAGE_NAME="${1:-localhost:5000/keylime-verifier:latest}"

echo "Building Keylime verifier from latest upstream/master..."
docker build -t "${IMAGE_NAME}" -f "$(dirname "$0")/Dockerfile" "$(dirname "$0")"

echo "Pushing image to ${IMAGE_NAME}..."
docker push "${IMAGE_NAME}"

echo ""
echo "✓ Built and pushed Keylime verifier image: ${IMAGE_NAME}"
echo ""
echo "To use this image, update deploy/helm/flightctl/values.yaml:"
echo ""
echo "  keylime:"
echo "    image:"
echo "      repository: ${IMAGE_NAME%:*}"
echo "      tag: ${IMAGE_NAME##*:}"
echo ""
