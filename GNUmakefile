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
