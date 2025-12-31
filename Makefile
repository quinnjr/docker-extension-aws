IMAGE ?= quinnjr/docker-aws-mfa
TAG ?= latest

BUILDER=buildx-multi-arch

.PHONY: build build-cross install uninstall clean prepare-buildx

# Install UI dependencies
ui/node_modules: ui/package.json
	cd ui && npm install

# Build for local architecture
build: ui/node_modules
	docker build -t $(IMAGE):$(TAG) .

# Build for multiple architectures
build-cross: ui/node_modules prepare-buildx
	docker buildx build \
		--builder $(BUILDER) \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(TAG) \
		--push \
		.

# Create buildx builder for multi-arch
prepare-buildx:
	docker buildx inspect $(BUILDER) > /dev/null 2>&1 || \
		docker buildx create --name $(BUILDER) --use --bootstrap

# Install the extension locally
install: build
	docker extension install $(IMAGE):$(TAG)

# Update the extension
update: build
	docker extension update $(IMAGE):$(TAG)

# Uninstall the extension
uninstall:
	docker extension rm $(IMAGE):$(TAG)

# Enable development mode (hot reload UI)
dev: build
	docker extension dev ui-source $(IMAGE):$(TAG) http://localhost:5173
	cd ui && npm run dev

# Reset development mode
dev-reset:
	docker extension dev reset $(IMAGE):$(TAG)

# View extension logs
logs:
	docker extension dev debug $(IMAGE):$(TAG)

# Clean build artifacts
clean:
	rm -rf ui/node_modules ui/dist
	docker rmi $(IMAGE):$(TAG) 2>/dev/null || true

# Validate extension
validate:
	docker extension validate $(IMAGE):$(TAG)

# Push to Docker Hub
push:
	docker push $(IMAGE):$(TAG)

# Tag and push release
release: build-cross
	@echo "Released $(IMAGE):$(TAG)"

help:
	@echo "Available targets:"
	@echo "  build       - Build extension for local architecture"
	@echo "  build-cross - Build for amd64 and arm64"
	@echo "  install     - Install extension in Docker Desktop"
	@echo "  update      - Update installed extension"
	@echo "  uninstall   - Remove extension from Docker Desktop"
	@echo "  dev         - Enable development mode with hot reload"
	@echo "  dev-reset   - Reset development mode"
	@echo "  logs        - View extension logs"
	@echo "  clean       - Remove build artifacts"
	@echo "  validate    - Validate extension"
	@echo "  push        - Push to Docker Hub"
	@echo "  release     - Build and push multi-arch release"
