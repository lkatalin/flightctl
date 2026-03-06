ifeq ($(DB_SIZE),)
	DB_SIZE := e2e
endif

ifeq ($(STATUS_UPDATE_INTERVAL),)
	STATUS_UPDATE_INTERVAL := 0m2s
endif

ifeq ($(SPEC_FETCH_INTERVAL),)
	SPEC_FETCH_INTERVAL := 0m2s
endif

# Create kind cluster if it doesn't exist (idempotent)
cluster: bin/e2e-certs/ca.pem
	test/scripts/install_kind.sh
	kind get clusters | grep kind || test/scripts/create_cluster.sh

clean-cluster:
	kind delete cluster

ifndef SKIP_BUILD
deploy: cluster build-containers build-cli deploy-helm prepare-agent-config
else
deploy: cluster deploy-helm prepare-agent-config
	@echo "Skipping container and CLI builds (SKIP_BUILD is set)"
endif

redeploy-api: flightctl-api-container
	test/scripts/redeploy.sh api

redeploy-worker: flightctl-worker-container
	test/scripts/redeploy.sh worker

redeploy-periodic: flightctl-periodic-container
	test/scripts/redeploy.sh periodic

redeploy-alert-exporter: flightctl-alert-exporter-container
	test/scripts/redeploy.sh alert-exporter

redeploy-alertmanager-proxy: flightctl-alertmanager-proxy-container
	test/scripts/redeploy.sh alertmanager-proxy

redeploy-telemetry-gateway: flightctl-telemetry-gateway-container
	test/scripts/redeploy.sh telemetry-gateway

redeploy-imagebuilder-worker: flightctl-imagebuilder-worker-container
	test/scripts/redeploy.sh imagebuilder-worker

redeploy-imagebuilder-api: flightctl-imagebuilder-api-container
	test/scripts/redeploy.sh imagebuilder-api


ifndef SKIP_BUILD
deploy-helm: flightctl-api-container flightctl-db-setup-container flightctl-worker-container flightctl-periodic-container flightctl-alert-exporter-container flightctl-alertmanager-proxy-container flightctl-imagebuilder-api-container flightctl-imagebuilder-worker-container flightctl-multiarch-cli-container flightctl-telemetry-gateway-container
endif
deploy-helm:
	kubectl config set-context kind-kind
	test/scripts/install_helm.sh
	test/scripts/deploy_with_helm.sh --db-size $(DB_SIZE)

prepare-agent-config:
	test/scripts/agent-images/prepare_agent_config.sh --status-update-interval $(STATUS_UPDATE_INTERVAL) --spec-fetch-interval $(SPEC_FETCH_INTERVAL)

prepare-agent-config-attestation:
	test/scripts/agent-images/prepare_agent_config.sh --status-update-interval $(STATUS_UPDATE_INTERVAL) --spec-fetch-interval $(SPEC_FETCH_INTERVAL) --tpm-attestation-enabled

deploy-db-helm: cluster
	test/scripts/deploy_with_helm.sh --only-db

deploy-db:
	sudo -E deploy/scripts/deploy_quadlet_service.sh db

deploy-kv:
	sudo -E deploy/scripts/deploy_quadlet_service.sh kv

deploy-alertmanager:
	sudo -E deploy/scripts/deploy_quadlet_service.sh alertmanager

deploy-alertmanager-proxy:
	sudo -E deploy/scripts/deploy_quadlet_service.sh alertmanager-proxy

# Can set the SKIP_BUILD variable to skip the build step and use existing containers
deploy-quadlets:
ifndef SKIP_BUILD
	$(MAKE) build-containers
	@echo "Copying containers from user to root context for systemd services..."
	podman save flightctl-api:latest | sudo podman load
	podman save flightctl-db-setup:latest | sudo podman load
	podman save flightctl-worker:latest | sudo podman load
	podman save flightctl-periodic:latest | sudo podman load
	podman save flightctl-alert-exporter:latest | sudo podman load
	podman save flightctl-cli-artifacts:latest | sudo podman load
	podman save flightctl-alertmanager-proxy:latest | sudo podman load
	podman save flightctl-pam-issuer:latest | sudo podman load
	podman save flightctl-imagebuilder-api:latest | sudo podman load
	podman save flightctl-imagebuilder-worker:latest | sudo podman load
	podman save flightctl-userinfo-proxy:latest | sudo podman load
	podman save flightctl-telemetry-gateway:latest | sudo podman load
endif
	$(MAKE) build-standalone
	sudo -E deploy/scripts/deploy_quadlets.sh

kill-db:
	sudo systemctl stop flightctl-db.service

kill-kv:
	sudo systemctl stop flightctl-kv.service

kill-alertmanager:
	sudo systemctl stop flightctl-alertmanager.service

kill-alertmanager-proxy:
	sudo systemctl stop flightctl-alertmanager-proxy.service

show-podman-secret:
	sudo podman secret inspect $(SECRET_NAME) --showsecret | jq '.[] | .SecretData'

# Can set the image tag to a specific version by using the PACKIT_CURRENT_VERSION variable
# which builds the rpm with the specified version.
#
# Example cmd:
# PACKIT_CURRENT_VERSION=latest make services-container
services-container: PACKIT_CURRENT_VERSION ?= latest
services-container: rpm
	@test -f bin/rpm/flightctl-services-*.rpm || (echo "No RPM file found - RPM build failed" && exit 1)
	sudo podman build -t flightctl-services:latest -f test/scripts/services-images/Containerfile.services .

run-services-container:
	@if ! sudo podman image exists localhost/flightctl-services:latest; then \
		echo "Container image not found, building flightctl-services container..."; \
		$(MAKE) services-container; \
	else \
		echo "Using existing flightctl-services container image"; \
	fi
	sudo podman run -d --privileged --replace \
	--name flightctl-services \
	-p 443:443 \
	-p 3443:3443 \
	-p 7443:7443 \
	-p 8090:8090 \
	-p 8443:8443 \
	-p 9093:9093 \
	localhost/flightctl-services:latest

clean-services-container:
	sudo podman stop flightctl-services || true
	sudo podman rm flightctl-services || true
	sudo podman rmi localhost/flightctl-services:latest || true

# Attestation demo deployment with official Keylime verifier
ifndef SKIP_BUILD
attestation-server: flightctl-api-container flightctl-db-setup-container flightctl-worker-container flightctl-periodic-container flightctl-alert-exporter-container flightctl-alertmanager-proxy-container flightctl-imagebuilder-api-container flightctl-imagebuilder-worker-container flightctl-multiarch-cli-container flightctl-telemetry-gateway-container
else
attestation-server:
	@echo "Skipping container builds (SKIP_BUILD is set)"
endif
attestation-server: cluster build-cli
	kubectl config set-context kind-kind
	test/scripts/install_helm.sh
	@echo "Loading custom Keylime verifier image into kind cluster..."
	@if podman image exists localhost/keylime_verifier:master-3a3cfa9; then \
		podman save localhost/keylime_verifier:master-3a3cfa9 | kind load image-archive /dev/stdin --name kind; \
		echo "✓ Keylime verifier image loaded successfully"; \
	else \
		echo "⚠ Warning: localhost/keylime_verifier:master-3a3cfa9 not found in podman images"; \
		echo "  The deployment may fail. Build it first or use the official image."; \
	fi
	@echo "Deploying with attestation demo configuration (custom Keylime verifier from master)..."
	EXTRA_VALUES_FILE=./deploy/helm/flightctl/values.attestation-demo.yaml test/scripts/deploy_with_helm.sh --db-size $(DB_SIZE)
	$(MAKE) configure-attestation-tpm-cas
	@echo ""
	@echo "=========================================="
	@echo "Attestation Server Deployed!"
	@echo "=========================================="
	@echo ""
	@echo "✓ FlightCTL API server configured for attestation"
	@echo "✓ Custom Keylime verifier (master branch) deployed and running"
	@echo "✓ TPM CA certificates configured"
	@echo ""
	@echo "Next step: Apply attestation policy with 'make attestation-policy'"

# Wait for FlightCTL API server to be ready and configure CLI
wait-for-server:
	@echo "Waiting for FlightCTL API server to be ready..."
	@kubectl rollout status deployment flightctl-api -n flightctl-external -w --timeout=300s
	@echo "Configuring FlightCTL CLI authentication..."
	@export FLIGHTCTL_NS=flightctl-external && \
		bash -c 'source test/scripts/functions && \
		for i in {1..60}; do \
			if try_login; then \
				echo "✓ CLI authenticated successfully"; \
				exit 0; \
			fi; \
			if [ $$i -eq 60 ]; then \
				echo "ERROR: Failed to authenticate CLI after 60 attempts"; \
				exit 1; \
			fi; \
			sleep 5; \
		done'
	@bash -c 'source test/scripts/functions && ensure_organization_set'
	@echo "✓ API server is ready and CLI is configured"

# Apply example attestation policy (allowlist format)
attestation-policy:
	@echo "Applying AttestationReference from measurements.txt..."
	bin/flightctl apply -f examples/attestation/attestation-reference-from-measurements.yaml
	@echo ""
	@echo "=========================================="
	@echo "Attestation Policy Applied!"
	@echo "=========================================="
	@echo ""
	@echo "✓ AttestationReference 'default-ima-policy' created"
	@echo "✓ All enrollments with attestation data will be verified against this policy"
	@echo ""
	@echo "Next step: Boot agent VM with 'make attestation-agent-vm'"
	@echo ""
	@echo "To use the full measurements.txt baseline instead:"
	@echo "  cd examples/attestation && ./create-attestation-ref-from-measurements.sh"
	@echo "  bin/flightctl apply -f examples/attestation/attestation-reference-from-measurements.yaml"

# Build agent-vm with the specific container image that matches measurements.txt
attestation-agent-vm:
	@echo "Building agent VM with attestation config and default bootc image..."
	@echo "Ensuring attestation-enabled config is generated and injected..."
	rm -f bin/.e2e-agent-injected
	$(MAKE) prepare-agent-config-attestation
	touch bin/.e2e-agent-certs
	@if [ ! -f bin/output/qcow2/disk.qcow2 ]; then \
		echo "Disk image not found, building e2e-agent-images..."; \
		$(MAKE) -j1 e2e-agent-images; \
	fi
	$(MAKE) -j1 agent-vm
	@echo ""
	@echo "=========================================="
	@echo "Agent VM Running!"
	@echo "=========================================="
	@echo ""
	@echo "✓ TPM-enabled agent VM running with vTPM 2.0 emulator"
	@echo "✓ Agent will automatically enroll with attestation data"
	@echo ""
	@echo "Monitor the agent enrollment and attestation verification in the server logs"

# Configure TPM CA certificates for attestation verification
configure-attestation-tpm-cas:
	@echo "Configuring TPM CA certificates for attestation verification..."
	test/scripts/add-certs-to-deployment.sh bin/tpm-cas

# Complete attestation demo: deploys server, applies policy, boots agent VM
attestation-demo: attestation-server wait-for-server attestation-policy attestation-agent-vm
	@echo ""
	@echo "=========================================="
	@echo "Attestation Demo Environment Ready!"
	@echo "=========================================="
	@echo ""
	@echo "✓ FlightCTL API server configured for attestation"
	@echo "✓ Custom Keylime verifier (master branch) deployed and running"
	@echo "✓ TPM CA certificates configured"
	@echo "✓ Attestation policy 'default-ima-policy' applied"
	@echo "✓ TPM-enabled agent VM running with vTPM 2.0 emulator"
	@echo ""
	@echo "The agent VM is now enrolling with attestation data."
	@echo "Monitor server logs to see attestation verification in action."
	@echo ""
	@echo "See examples/attestation/README.md for more details"
	@echo ""

# Backward compatibility aliases
attestation-demo-deploy-helm: attestation-server
attestation-demo-agent-vm: attestation-agent-vm
attestation-demo-apply-policy: attestation-policy

clean-attestation-demo: clean-agent-vm clean-cluster

# Rebuild agent from source with cache clearing to pick up code changes
attestation-demo-rebuild-agent: attestation-server wait-for-server
	@echo "Step 1: Clearing all build caches to force rebuild from source..."
	@echo "  - Clearing Go build cache..."
	@go clean -cache
	@echo "  - Removing RPM files and build artifacts..."
	@rm -rf bin/rpm/
	@rm -rf bin/.rpm
	@echo "  - Removing agent artifacts..."
	@rm -rf bin/agent-artifacts/
	@rm -f bin/.e2e-agent-images-*
	@rm -f bin/.e2e-agent-injected
	@rm -f bin/.e2e-agent-certs
	@echo "  - Removing qcow2 disk..."
	@rm -f bin/output/qcow2/disk.qcow2 || true
	@echo ""
	@echo "Step 2: Rebuilding agent RPM and disk image from source..."
	$(MAKE) e2e-agent-images
	@echo ""
	@echo "Step 3: Generating new measurements.txt from rebuilt image..."
	@echo "  (This happens automatically during e2e-agent-images)"
	@echo ""
	@echo "Step 4: Creating AttestationReference from new measurements..."
	examples/attestation/create-attestation-ref-from-measurements.sh
	@echo ""
	@echo "Step 5: Applying updated AttestationReference..."
	bin/flightctl apply -f examples/attestation/attestation-reference-from-measurements.yaml
	@echo ""
	@echo "Step 6: Starting agent VM..."
	$(MAKE) attestation-agent-vm
	@echo ""
	@echo "==========================================="
	@echo "Agent Rebuilt and Running!"
	@echo "==========================================="
	@echo ""
	@echo "✓ Agent rebuilt from latest source code"
	@echo "✓ New measurements.txt generated"
	@echo "✓ AttestationReference updated with new measurements"
	@echo "✓ Agent VM running with vTPM"
	@echo ""
	@echo "Monitor attestation in server logs"

# Apply attestation policy from measurements.txt
attestation-demo-apply-policy:
	@echo "Applying AttestationReference from measurements.txt..."
	bin/flightctl apply -f examples/attestation/attestation-reference-from-measurements.yaml
	@echo ""
	@echo "AttestationReference 'default-ima-policy' created!"
	@echo "All enrollments with attestation data will be verified against this policy."
	@echo ""
	@echo "Using measurements from measurements.txt (automatically generated from disk image)"

PHONY: deploy-db deploy cluster services-container run-services-container clean-services-container attestation-server wait-for-server attestation-policy attestation-agent-vm attestation-demo prepare-agent-config-attestation configure-attestation-tpm-cas clean-attestation-demo attestation-demo-rebuild-agent attestation-demo-deploy-helm attestation-demo-apply-policy
