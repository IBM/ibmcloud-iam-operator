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
	#docker push ${IMG}:${TAG}

.PHONY: run
run:
	operator-sdk up local --namespace=${OPERATOR_NAMESPACE}

.PHONY: install
install: 
	kubectl apply -f deploy/namespace.yaml
	hack/config-operator.sh ${DEF_NAMESPACE}
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_v1alpha1_accesspolicy_crd.yaml
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_v1alpha1_accessgroup_crd.yaml
	kubectl apply -f deploy/crds/ibmcloud.ibm.com_v1alpha1_customrole_crd.yaml
	kubectl apply -f deploy/service_account.yaml 
	kubectl apply -f deploy/role.yaml 
	kubectl apply -f deploy/role_binding.yaml 
	kubectl apply -f deploy/operator.yaml 

.PHONY: uninstall
uninstall:
	kubectl delete -f deploy/crds/ibmcloud.ibm.com_v1alpha1_accesspolicy_crd.yaml
	kubectl delete  -f deploy/crds/ibmcloud.ibm.com_v1alpha1_accessgroup_crd.yaml
	kubectl delete  -f deploy/crds/ibmcloud.ibm.com_v1alpha1_customrole_crd.yaml
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
	hack/config-operator.sh ${OPERATOR_NAMESPACE}
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

# Generate code
generate:
	go generate ./pkg/... ./cmd/...
	hack/update-codegen.sh

# Run the operator-sdk scorecard on latest release
scorecard:
	hack/operator-scorecard.sh 

MAKEFLAGS += --no-print-directory
export OP_PATH=''
export CLEAN_MODE=NORMAL
export KUBECONFIG ?= ${HOME}/.kube/config
export KUBE_VER ?= v1.17.0
export OLM_VER ?= 0.14.1
export SDK_VER ?= v0.16.0
export VERBOSE ?= 0
export NO_KIND ?= 0
export KIND_VER ?= v0.7.0
export OPERATOR_TESTING_IMAGE := cloudoperators/ibmcloud-iam-operator:0.1.0

help:
	@grep -E '^[a-zA-Z0-9/._-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

check_path:
	@if [ ! -d ${OP_PATH} ]; then echo "Operator path not found you need set it with OP_PATH=upstream-community-operators/your-operator"; exit 1; fi

check_no_kind :
	@if [[ "${NO_KIND}" != "0" && -z "${CATALOG_IMAGE}" ]] ; then echo "You specified NO_KIND but did not specify CATALOG_IMAGE"; exit 1; elif [[ "${NO_KIND}" == "0" && ! -z "${CATALOG_IMAGE}" ]] ; then echo "You specified CATALOG_IMAGE but it will be ignore until you set NO_KIND to 1" ; fi

check_kind_install :
	@if [[ "${NO_KIND}" == "0" &&  "$(which kind)" =~ "not found" ]] ; then echo "KIND (https://kind.sigs.k8s.io) is not installed and NO_KIND is not set." ; exit 1 ; fi

force_pull_image:
	@scripts/ci/run-script "docker pull ${OPERATOR_TESTING_IMAGE}" "Pulling docker image"

minikube.install: ## Install the local minikube
	@./scripts/ci/install-minikube
	@echo "Installed"

kind.install:
	@scripts/ci/run-script "scripts/ci/install-kind" "Install KIND"

kind.start:
	@scripts/ci/run-script "scripts/ci/start-kind" "Start KIND"

minikube.start: ## Start local minikube
	@scripts/ci/run-script "scripts/ci/start-minikube" "Start minikube"

olm.install: force_pull_image ## Install OLM to your cluster
	@python3 scripts/utils/check-kube-config.py
	@docker run --network=host -v ${KUBECONFIG}:/root/.kube/config:z -v ${PWD}/community-operators:/community-operators:z -v ${PWD}/upstream-community-operators:/upstream-community-operators:z -it ${OPERATOR_TESTING_IMAGE} olm.install --no-print-directory VERBOSE=${VERBOSE}

operator.install: check_path check_no_kind check_kind_install force_pull_image
	@python3 scripts/utils/check-kube-config.py
	@scripts/ci/run-script "scripts/ci/build-catalog-image" "Building catalog image"
	@docker run --network=host -v ${KUBECONFIG}:/root/.kube/config:z -v ${PWD}/community-operators:/community-operators:z -v ${PWD}/upstream-community-operators:/upstream-community-operators:z -ti ${OPERATOR_TESTING_IMAGE} operator.install --no-print-directory OP_PATH=${OP_PATH} VERBOSE=${VERBOSE} OP_VER=${OP_VER} OP_CHANNEL=${OP_CHANNEL} INSTALL_MODE=${INSTALL_MODE} CLEAN_MODE=${CLEAN_MODE} CATALOG_IMAGE=${CATALOG_IMAGE} NO_KIND=${NO_KIND}

operator.cleanup: check_path check_no_kind check_kind_install 
	@scripts/ci/run-script "scripts/ci/cleanup" "Cleaning"

operator.test: check_path check_no_kind check_kind_install force_pull_image ## Operator test which run courier and scorecard
	@python3 scripts/utils/check-kube-config.py
	@scripts/ci/run-script "scripts/ci/build-catalog-image" "Building catalog image"
	@docker run --network=host -v ${KUBECONFIG}:/root/.kube/config:z -v ${PWD}/community-operators:/community-operators:z -v ${PWD}/upstream-community-operators:/upstream-community-operators:z -ti ${OPERATOR_TESTING_IMAGE} operator.test --no-print-directory OP_PATH=${OP_PATH} VERBOSE=${VERBOSE} OP_VER=${OP_VER} OP_CHANNEL=${OP_CHANNEL} INSTALL_MODE=${INSTALL_MODE} CLEAN_MODE=${CLEAN_MODE} OLM_VER=${OLM_VER} KUBE_VER=${KUBE_VER} NO_KIND=${NO_KIND} CATALOG_IMAGE=${CATALOG_IMAGE}

operator.verify: check_path force_pull_image ## Run only courier
	@docker run -v ${PWD}/community-operators:/community-operators:z -v ${PWD}/upstream-community-operators:/upstream-community-operators:z -ti ${OPERATOR_TESTING_IMAGE} operator.verify --no-print-directory OP_PATH=${OP_PATH} VERBOSE=${VERBOSE}