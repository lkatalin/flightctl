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
	@echo "Deploying with attestation demo configuration (Keylime verifier from quay.io/keylime/keylime_verifier:master)..."
	EXTRA_VALUES_FILE=./deploy/helm/flightctl/values.attestation-demo.yaml test/scripts/deploy_with_helm.sh --db-size $(DB_SIZE)
	$(MAKE) configure-attestation-tpm-cas
	@echo ""
	@echo "=========================================="
	@echo "Attestation Server Deployed!"
	@echo "=========================================="
	@echo ""
	@echo "✓ FlightCTL API server configured for attestation"
	@echo "✓ Keylime verifier (master branch) deployed and running"
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
	bin/flightctl apply -f examples/attestation/attestation-reference-20260311-132737.yaml
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
	@echo "  bin/flightctl apply -f examples/attestation/attestation-reference-20260311-132737.yaml"

# Build agent-vm with the specific container image that matches measurements.txt
# Automatically detects CA certificate changes and re-injects config when needed
attestation-agent-vm:
	@echo "Building agent VM with attestation config and default bootc image..."
	@echo "Ensuring attestation-enabled config is generated and injected..."
	$(MAKE) prepare-agent-config-attestation
	touch bin/.e2e-agent-certs
	@if [ ! -f bin/output/qcow2/disk.qcow2 ]; then \
		echo "Disk image not found, building e2e-agent-images..."; \
		$(MAKE) -j1 AGENT_OS_ID=cs9-bootc e2e-agent-images; \
		echo "Injecting attestation config into new disk..."; \
		rm -f bin/.e2e-agent-injected; \
		$(MAKE) prepare-e2e-qcow-config; \
		echo "Booting VM with injected config..."; \
		$(MAKE) -j1 agent-vm INJECT_CONFIG=false; \
	elif ! test/scripts/check_attestation_in_qcow.sh bin/output/qcow2/disk.qcow2 2>/dev/null; then \
		echo "Disk exists but lacks attestation config, injecting..."; \
		rm -f bin/.e2e-agent-injected; \
		$(MAKE) prepare-e2e-qcow-config; \
		echo "Booting VM with injected config..."; \
		$(MAKE) -j1 agent-vm INJECT_CONFIG=false; \
	elif [ -f bin/agent/etc/flightctl/config.yaml ] && [ ! -f bin/.e2e-agent-injected -o bin/agent/etc/flightctl/config.yaml -nt bin/.e2e-agent-injected ]; then \
		echo "Agent config updated since last injection, re-injecting config..."; \
		rm -f bin/.e2e-agent-injected; \
		$(MAKE) prepare-e2e-qcow-config; \
		echo "Booting VM with updated config..."; \
		$(MAKE) -j1 agent-vm INJECT_CONFIG=false; \
	else \
		echo "Disk has current agent config, booting without re-injection..."; \
		$(MAKE) -j1 agent-vm INJECT_CONFIG=false; \
	fi
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
	@# Create bin/tpm-cas directory and populate with test swtpm CA if it doesn't exist
	@if [ ! -d bin/tpm-cas ] || [ -z "$$(ls -A bin/tpm-cas/*.pem 2>/dev/null)" ]; then \
		echo "Creating bin/tpm-cas directory and generating test swtpm CA certificates..."; \
		mkdir -p bin/tpm-cas bin/swtpm-ca; \
		test/scripts/create-test-swtpm-ca.sh bin/swtpm-ca; \
		cp bin/swtpm-ca/swtpm-localca-rootca-cert.pem bin/tpm-cas/; \
		cp bin/swtpm-ca/issuercert.pem bin/tpm-cas/swtpm-localca-issuer-cert.pem; \
		echo "✓ Test swtpm CA certificates created in bin/tpm-cas/"; \
	else \
		echo "✓ TPM CA certificates already exist in bin/tpm-cas/"; \
	fi
	test/scripts/add-certs-to-deployment.sh bin/tpm-cas

# Attestation server + policy (no agent VM)
# Automatically generates agent config after server is ready
attestation-server-policy: attestation-server wait-for-server prepare-agent-config-attestation attestation-policy
	@echo ""
	@echo "=========================================="
	@echo "Attestation Server Ready!"
	@echo "=========================================="
	@echo ""
	@echo "✓ FlightCTL API server configured for attestation"
	@echo "✓ Custom Keylime verifier (master branch) deployed and running"
	@echo "✓ TPM CA certificates configured"
	@echo "✓ Agent config generated with current server certificates"
	@echo "✓ Attestation policy 'default-ima-policy' applied"
	@echo ""
	@echo "Next step: Boot agent VM with 'make attestation-agent-vm'"
	@echo ""

# Complete attestation demo: server + policy + agent VM
attestation-demo: attestation-server-policy attestation-agent-vm
	@echo ""
	@echo "=========================================="
	@echo "Complete Attestation Demo Running!"
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

# Clean only the attestation server (cluster), preserve all agent artifacts
clean-attestation-server: clean-cluster
	@echo "Attestation server cleaned (agent artifacts preserved)"

# Clean both server and agent VM, but preserve disk image and measurements
# Use this for redeploying the demo without rebuilding the agent
clean-attestation-demo: clean-agent-vm clean-attestation-server
	@echo ""
	@echo "=========================================="
	@echo "Attestation Demo Cleaned"
	@echo "=========================================="
	@echo ""
	@echo "✓ Server cluster removed"
	@echo "✓ Agent VM stopped"
	@echo "✓ Disk image preserved: bin/output/qcow2/disk.qcow2"
	@echo "✓ Measurements preserved: examples/attestation/measurements-*.txt"
	@echo ""
	@echo "Run 'make attestation-demo' to redeploy with existing artifacts"

# Rebuild agent disk image from source with new measurements (no policy apply)
rebuild-attestation-agent-disk:
	@echo ""
	@echo "=========================================="
	@echo "Rebuilding Agent Disk Image"
	@echo "=========================================="
	@echo ""
	@echo "Step 1: Clearing Go build cache..."
	@go clean -cache
	@echo ""
	@echo "Step 2: Removing agent bundle, RPM, disk image, and build caches..."
	rm -rf bin/agent-artifacts/
	rm -rf bin/rpm/flightctl-agent-*.rpm
	rm -rf bin/.rpm
	rm -rf bin/osbuild-cache/
	rm -rf bin/output/
	rm -f bin/.e2e-agent-images-*
	rm -f bin/.e2e-agent-certs
	rm -f bin/.e2e-agent-injected
	@echo ""
	@echo "Step 3: Rebuilding agent RPM and disk image from source..."
	$(MAKE) AGENT_OS_ID=cs9-bootc e2e-agent-images
	@echo ""
	@echo "Step 4: Creating AttestationReference from new measurements..."
	examples/attestation/create-attestation-ref-from-measurements.sh
	@echo ""
	@echo "=========================================="
	@echo "Agent Disk Rebuilt!"
	@echo "=========================================="
	@echo ""
	@echo "✓ New disk image: bin/output/qcow2/disk.qcow2"
	@echo "✓ New measurements: examples/attestation/measurements-*.txt"
	@echo "✓ AttestationReference updated"
	@echo ""
	@echo "Next step: Apply policy with 'make apply-attestation-policy'"

# Rebuild agent disk + apply policy (assumes server is running)
rebuild-attestation-agent-disk-policy: rebuild-attestation-agent-disk
	@echo ""
	@echo "Applying updated AttestationReference..."
	bin/flightctl apply -f examples/attestation/attestation-reference-from-measurements.yaml
	@echo ""
	@echo "✓ AttestationReference applied successfully"

# Complete agent rebuild workflow: disk + policy + VM (assumes server is running)
rebuild-attestation-agent: rebuild-attestation-agent-disk-policy
	@echo ""
	@echo "Starting agent VM with rebuilt disk..."
	$(MAKE) attestation-agent-vm
	@echo ""
	@echo "==========================================="
	@echo "Agent Rebuilt and Running!"
	@echo "==========================================="
	@echo ""
	@echo "✓ Agent disk rebuilt from source"
	@echo "✓ New measurements extracted"
	@echo "✓ AttestationReference applied"
	@echo "✓ Agent VM running with vTPM"
	@echo ""
	@echo "Monitor server logs to verify attestation"

# Alias for applying attestation policy
apply-attestation-policy: attestation-policy

# Rebuild qcow2 disk image from existing container bundle (for reproducibility testing)
# This loads the bundle into podman and runs bootc-image-builder to create a fresh qcow2
# Usage: make rebuild-qcow2-from-bundle [AGENT_OS_ID=cs9-bootc]
rebuild-qcow2-from-bundle: AGENT_OS_ID ?= cs9-bootc
rebuild-qcow2-from-bundle:
	@BUNDLE_PATH="bin/agent-artifacts/agent-images-bundle-$(AGENT_OS_ID).tar"; \
	if [ ! -f "$$BUNDLE_PATH" ]; then \
		echo "Error: Container bundle not found at $$BUNDLE_PATH"; \
		echo "Run 'make AGENT_OS_ID=$(AGENT_OS_ID) e2e-agent-images' first to create the bundle."; \
		exit 1; \
	fi; \
	echo "Loading container images from bundle into root podman context..."; \
	sudo podman load -i "$$BUNDLE_PATH" >/dev/null; \
	echo ""; \
	echo "Finding base image from bundle..."; \
	BASE_IMAGE=$$(sudo podman images --filter "label=io.flightctl.e2e.component=device" --format "{{.Repository}}:{{.Tag}}" | grep "base-$(AGENT_OS_ID)" | head -1); \
	if [ -z "$$BASE_IMAGE" ]; then \
		echo "Error: No base-$(AGENT_OS_ID) image found in bundle"; \
		exit 1; \
	fi; \
	echo "Using base image: $$BASE_IMAGE"; \
	echo ""; \
	echo "Removing old qcow2 if it exists..."; \
	sudo rm -rf bin/output/agent-qcow2-$(AGENT_OS_ID); \
	rm -f bin/output/qcow2/disk.qcow2; \
	echo ""; \
	echo "Rebuilding qcow2 from container image (this may take 5-10 minutes)..."; \
	mkdir -p bin/output/agent-qcow2-$(AGENT_OS_ID); \
	mkdir -p bin/dnf-cache bin/osbuild-cache; \
	sudo podman run --rm \
		-it \
		--privileged \
		--pull=newer \
		--security-opt label=type:unconfined_t \
		-v "$$(pwd)/bin/output/agent-qcow2-$(AGENT_OS_ID)":/output \
		-v "$$(pwd)/bin/dnf-cache":/var/cache/dnf:Z \
		-v "$$(pwd)/bin/osbuild-cache":/var/cache/osbuild:Z \
		-v /var/lib/containers/storage:/var/lib/containers/storage \
		quay.io/centos-bootc/bootc-image-builder:latest \
		build \
		--type qcow2 \
		"$$BASE_IMAGE"; \
	sudo chown -R "$$(id -un)":"$$(id -gn)" "bin/output/agent-qcow2-$(AGENT_OS_ID)"; \
	if [ ! -f "bin/output/agent-qcow2-$(AGENT_OS_ID)/qcow2/disk.qcow2" ]; then \
		echo "Error: qcow2 build failed - disk image not created"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Moving qcow2 to standard location..."; \
	mkdir -p bin/output/qcow2; \
	mv bin/output/agent-qcow2-$(AGENT_OS_ID)/qcow2/disk.qcow2 bin/output/qcow2/disk.qcow2; \
	echo ""; \
	echo "=========================================="; \
	echo "QCOW2 Rebuilt from Bundle!"; \
	echo "=========================================="; \
	echo ""; \
	echo "✓ Disk image: bin/output/qcow2/disk.qcow2"; \
	echo ""; \
	echo "Next step: Extract measurements and compare with baseline"; \
	echo "  examples/attestation/extract-measurements-from-disk.sh"

# Clean everything including disk image (alias for backward compatibility)
clean-attestation-demo-full: clean-agent-vm clean-cluster
	@echo "Removing agent bundle, RPM, and build caches..."
	rm -rf bin/agent-artifacts/
	rm -rf bin/rpm/flightctl-agent-*.rpm
	rm -rf bin/.rpm
	rm -rf bin/osbuild-cache/
	rm -rf bin/output/
	rm -f bin/.e2e-agent-images-*
	rm -f bin/.e2e-agent-certs
	rm -f bin/.e2e-agent-injected
	@echo ""
	@echo "=========================================="
	@echo "Full Clean Complete"
	@echo "=========================================="
	@echo ""
	@echo "All agent artifacts removed."
	@echo "Use 'make rebuild-attestation-agent-disk' to rebuild the agent disk image."

# Full rebuild workflow: server + agent rebuild + policy + VM
# This deploys a fresh server AND rebuilds the agent from source
# Use this for "Option B" testing (does rebuilding produce matching measurements?)
attestation-demo-rebuild-agent: attestation-server-policy
	@echo ""
	@echo "=========================================="
	@echo "Full Rebuild: Server + Agent"
	@echo "=========================================="
	@echo ""
	@echo "Server deployed. Now rebuilding agent from source..."
	@echo ""
	$(MAKE) rebuild-attestation-agent
	@echo ""
	@echo "==========================================="
	@echo "Full Rebuild Complete!"
	@echo "==========================================="
	@echo ""
	@echo "✓ Fresh server deployed with attestation"
	@echo "✓ Agent rebuilt from latest source code"
	@echo "✓ New measurements extracted"
	@echo "✓ Updated AttestationReference applied"
	@echo "✓ Agent VM running with vTPM"
	@echo ""
	@echo "Monitor server logs to verify attestation (expecting zero errors)"

# Apply attestation policy from measurements.txt
attestation-demo-apply-policy:
	@echo "Applying AttestationReference from measurements.txt..."
	bin/flightctl apply -f examples/attestation/attestation-reference-20260311-132737.yaml
	@echo ""
	@echo "AttestationReference 'default-ima-policy' created!"
	@echo "All enrollments with attestation data will be verified against this policy."
	@echo ""
	@echo "Using measurements from measurements.txt (automatically generated from disk image)"

PHONY: deploy-db deploy cluster services-container run-services-container clean-services-container attestation-server wait-for-server attestation-policy apply-attestation-policy attestation-agent-vm attestation-server-policy attestation-demo prepare-agent-config-attestation configure-attestation-tpm-cas clean-attestation-server clean-attestation-demo clean-attestation-demo-full rebuild-attestation-agent-disk rebuild-attestation-agent-disk-policy rebuild-attestation-agent attestation-demo-rebuild-agent attestation-demo-deploy-helm attestation-demo-apply-policy rebuild-qcow2-from-bundle
