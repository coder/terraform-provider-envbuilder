default: testacc

test: testacc

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

fmt: examples/*/*.tf
	terraform fmt -recursive

gen:
	go generate

build: terraform-provider-envbuilder

terraform-provider-envbuilder: internal/provider/*.go main.go
	CGO_ENABLED=0 go build .

.PHONY: update-envbuilder-version
update-envbuilder-version:
	go get github.com/coder/envbuilder@main
	go mod tidy

# Starts a local Docker registry on port 5000 with a local disk cache.
.PHONY: test-registry
test-registry: test-registry-container test-images-pull test-images-push

.PHONY: test-registry-container
test-registry-container: .registry-cache
	if ! curl -fsSL http://localhost:5000/v2/_catalog > /dev/null 2>&1; then \
		docker rm -f tfprov-envbuilder-registry && \
		docker run -d -p 5000:5000 --name envbuilder-registry --volume $(PWD)/.registry-cache:/var/lib/registry registry:2; \
	fi

# Pulls images referenced in integration tests and pushes them to the local cache.
.PHONY: test-images-push
test-images-push: .registry-cache/docker/registry/v2/repositories/test-ubuntu

.PHONY: test-images-pull
test-images-pull:
	docker pull ubuntu:latest
	docker tag ubuntu:latest localhost:5000/test-ubuntu:latest

.registry-cache:
	mkdir -p .registry-cache && chmod -R ag+w .registry-cache

.registry-cache/docker/registry/v2/repositories/test-ubuntu:
	docker push localhost:5000/test-ubuntu:latest
