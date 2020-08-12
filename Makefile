# Image URL to use all building/pushing image targets
IMG ?= cloudoperators/ibmcloud-iam-operator
TAG ?= 0.1.0
DEF_NAMESPACE ?= default
OPERATOR_NAMESPACE ?= ibmcloud-iam-operators
GOFILES = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

.PHONY: codegen
codegen:
	operator-sdk generate k8s
	operator-sdk generate crds

.PHONY: build
build: 
	operator-sdk build ${IMG}:${TAG}
	docker push ${IMG}:${TAG}

.PHONY: run
run:
	operator-sdk up local --namespace=${OPERATOR_NAMESPACE}

.PHONY: install
install: 
	kubectl apply -f deploy/namespace.yaml
	hack/config-operator.sh
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_accesspolicies_crd.yaml
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_accessgroups_crd.yaml
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_customroles_crd.yaml
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_authorizationpolicies_crd.yaml
	kubectl apply -f deploy/service_account.yaml 
	kubectl apply -f deploy/role.yaml 
	kubectl apply -f deploy/role_binding.yaml 
	kubectl apply -f deploy/operator.yaml 

.PHONY: uninstall
uninstall:
	kubectl delete -f deploy/crds/ibmcloud.ibm.com_accesspolicies_crd.yaml
	kubectl delete  -f deploy/crds/ibmcloud.ibm.com_accessgroups_crd.yaml
	kubectl delete  -f deploy/crds/ibmcloud.ibm.com_customroles_crd.yaml
	kubectl delete -f deploy/crds/ibmcloud.ibm.com_authorizationpolicies_crd.yaml
	kubectl delete -f deploy/role.yaml 
	kubectl delete -f deploy/role_binding.yaml
	kubectl delete -f deploy/service_account.yaml
	kubectl delete -f deploy/operator.yaml
	kubectl delete -f deploy/namespace.yaml
	
.PHONY: unittest
unittest:
	go test -v ./pkg/... -mod vendor -coverprofile coverage.out
	go tool cover -func coverage.out
	go tool cover -html coverage.out
	
.PHONY: e2etest
e2etest:
	test/e2e/test.sh
	kubectl apply -f deploy/namespace.yaml
	hack/config-operator.sh
	operator-sdk test local ./test/e2e --go-test-flags "-v" --namespace ${OPERATOR_NAMESPACE}
	kubectl delete -f deploy/namespace.yaml

.PHONY: e2etest-local
e2etest-local:
	kubectl apply -f deploy/namespace.yaml
	operator-sdk test local ./test/e2e --go-test-flags "-v" --namespace ${OPERATOR_NAMESPACE} --up-local
	kubectl delete -f deploy/namespace.yaml

check-tag:
ifndef TAG
	$(error TAG is undefined! Please set TAG to the latest release tag, using the format x.y.z e.g. export TAG=0.1.1 ) 
endif

# make an initial release for olm and releases
release: check-tag
	python hack/package.py v${TAG}

# make a future release for olm and releases
release-update: check-tag
	python hack/package.py v${TAG} --is_update

# Run the operator-sdk scorecard on latest release
scorecard:
	hack/operator-scorecard.sh 