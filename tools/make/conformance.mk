##@ Gateway API conformance
# See tests/conformance/README.md for what these do and why.

# Gateway API release whose CRDs are installed and whose suite runs. Keep in step with tests/conformance/go.mod.
CONFORMANCE_GATEWAY_API_VERSION ?= v1.7.0
CONFORMANCE_GATEWAY_CLASS ?= ngrok
CONFORMANCE_PROFILES ?= GATEWAY-HTTP
# Extended features we claim. Core features (Gateway, HTTPRoute, ReferenceGrant) come with the profile.
CONFORMANCE_SUPPORTED_FEATURES ?= Gateway,HTTPRoute,ReferenceGrant
CONFORMANCE_SKIP_TESTS ?=
# Run a single test, e.g. CONFORMANCE_RUN_TEST=HTTPRouteSimpleSameNamespace
CONFORMANCE_RUN_TEST ?=
# Domain the operator generates names under for listeners without a hostname. The account must be able
# to reserve names under it.
CONFORMANCE_GENERATED_DOMAIN_SUFFIX ?= ngrok.app
# Appended to the suite's hostnames (example.com) so the account can reserve them. .test is reserved by
# RFC 2606 and never resolves, so these names can't collide with anyone's real domain. The user name keeps
# two people's runs from claiming the same names.
CONFORMANCE_HOSTNAME_SUFFIX ?= $(shell whoami | tr -cd 'a-z0-9-').gwapi-conformance.test
# Version written into the report. A submitted report must name a released version.
CONFORMANCE_OPERATOR_VERSION ?= $(shell git describe --tags --match 'ngrok-operator-*' --always | sed 's/^ngrok-operator-//')
CONFORMANCE_REPORT ?= $(CURDIR)/tests/conformance/reports/standard-$(CONFORMANCE_OPERATOR_VERSION)-default-report.yaml

.PHONY: conformance
conformance: conformance-setup conformance-test ## Install the Gateway API CRDs and the operator in kind, then run the conformance suite.

.PHONY: conformance-setup
conformance-setup: _deploy-check-env-vars ## Install the Gateway API CRDs and deploy the operator configured for the conformance suite.
	@kind get clusters | grep -qx "$(KIND_CLUSTER_NAME)" || $(MAKE) kind-create
	kubectl apply --server-side --force-conflicts -f https://github.com/kubernetes-sigs/gateway-api/releases/download/$(CONFORMANCE_GATEWAY_API_VERSION)/standard-install.yaml
	$(MAKE) deploy_gateway HELM_EXTRA_FLAGS="--set ngrok.features.gateway.generatedDomainSuffix=$(CONFORMANCE_GENERATED_DOMAIN_SUFFIX) --set ngrok.features.gateway.conformanceHostnameSuffix=$(CONFORMANCE_HOSTNAME_SUFFIX)"
	kubectl -n $(KUBE_NAMESPACE) rollout status deploy/$(KUBE_DEPLOYMENT_NAME) --timeout=180s
	printf 'apiVersion: gateway.networking.k8s.io/v1\nkind: GatewayClass\nmetadata:\n  name: %s\nspec:\n  controllerName: ngrok.com/gateway-controller\n' "$(CONFORMANCE_GATEWAY_CLASS)" | kubectl apply -f -
	kubectl wait --for=condition=Accepted gatewayclass/$(CONFORMANCE_GATEWAY_CLASS) --timeout=60s
	sed 's/CONFORMANCE_HOSTNAME_SUFFIX/$(CONFORMANCE_HOSTNAME_SUFFIX)/' tests/conformance/catch-all.yaml | kubectl apply -f -

.PHONY: conformance-test
conformance-test: ## Run the Gateway API conformance suite against the operator in the current kube context.
	@mkdir -p $(dir $(CONFORMANCE_REPORT))
	cd tests/conformance && CONFORMANCE_HOSTNAME_SUFFIX=$(CONFORMANCE_HOSTNAME_SUFFIX) \
		go test -v -count=1 -timeout 120m -run TestConformance ./ -args \
		--gateway-class=$(CONFORMANCE_GATEWAY_CLASS) \
		--conformance-profiles=$(CONFORMANCE_PROFILES) \
		--supported-features=$(CONFORMANCE_SUPPORTED_FEATURES) \
		--organization=ngrok \
		--project=ngrok-operator \
		--url=https://github.com/ngrok/ngrok-operator \
		--version=$(CONFORMANCE_OPERATOR_VERSION) \
		--contact=https://github.com/ngrok/ngrok-operator/issues/new/choose \
		--mode=default \
		--report-output=$(CONFORMANCE_REPORT) \
		$(if $(CONFORMANCE_SKIP_TESTS),--skip-tests=$(CONFORMANCE_SKIP_TESTS)) \
		$(if $(CONFORMANCE_RUN_TEST),--run-test=$(CONFORMANCE_RUN_TEST))
