##@ Deploying

ifneq ($(CODESPACE_NAME),)
HELM_DESCRIPTION_FLAG = --set-string ngrok.description="codespace: $(CODESPACE_NAME)"
endif

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/api/main.go


.PHONY: deploy
deploy: _deploy-check-env-vars docker-build manifests _helm_setup kind-load-image ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set-string apiManager.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set apiManager.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set-string agent.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set agent.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set ngrok.log.format=console \
		--set-string ngrok.log.level="8" \
		--set ngrok.log.stacktraceLevel=panic \
		--set ngrok.metadata.env=local,ngrok.metadata.from=makefile \
		--set features.drainPolicy="Delete" \
		$(HELM_DESCRIPTION_FLAG)

.PHONY: deploy_gateway
deploy_gateway: _deploy-check-env-vars docker-build manifests _helm_setup kind-load-image ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set-string apiManager.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set apiManager.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set-string agent.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set agent.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set ngrok.log.format=console \
		--set-string ngrok.log.level="8" \
		--set ngrok.log.stacktraceLevel=panic \
		--set ngrok.metadata.env=local,ngrok.metadata.from=makefile \
		--set features.gateway.enabled=true \
		--set features.drainPolicy="Delete" \
		$(HELM_DESCRIPTION_FLAG)

.PHONY: deploy_with_bindings
deploy_with_bindings: _deploy-check-env-vars docker-build manifests _helm_setup kind-load-image ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set-string apiManager.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set apiManager.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set-string agent.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set agent.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set-string bindingsForwarder.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set bindingsForwarder.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set ngrok.log.format=console \
		--set ngrok.log.level=debug \
		--set ngrok.log.stacktraceLevel=panic \
		--set ngrok.metadata.env=local,ngrok.metadata.from=makefile \
		--set features.bindings.enabled=true \
		--set features.drainPolicy="Delete" \
		$(HELM_DESCRIPTION_FLAG)

.PHONY: deploy_for_e2e
deploy_for_e2e: _deploy-check-env-vars docker-build manifests _helm_setup kind-load-image ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set apiManager.config.oneClickDemoMode=$(DEPLOY_ONE_CLICK_DEMO_MODE) \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set image.pullPolicy="Never" \
		--set-string apiManager.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set apiManager.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"e2e\"\}" \
		--set-string agent.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set agent.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"e2e\"\}" \
		--set-string bindingsForwarder.podAnnotations."redeployTimestamp"="$(DEPLOY_ROLLOUT_TIMESTAMP)" \
		--set bindingsForwarder.podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"e2e\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set ngrok.log.format=console \
		--set ngrok.log.level=debug \
		--set ngrok.log.stacktraceLevel=panic \
		--set ngrok.metadata.env=local,ngrok.metadata.from=makefile \
		--set features.bindings.enabled=true \
		--set features.bindings.serviceAnnotations.annotation1="val1" \
		--set features.bindings.serviceAnnotations.annotation2="val2" \
		--set features.bindings.serviceLabels.label1="val1" \
		--set features.drainPolicy="Delete"

.PHONY: deploy_multi_namespace
## 1. We want to install the CRDs only once at the beginning
## 2. We want to make a namespace-a and namespace-b
## 3. We want to install the helm chart twice, watching only the namespace belonging to each one
deploy_multi_namespace: _deploy-check-env-vars docker-build manifests _helm_setup kind-load-image ## Deploy multiple copies of the controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade ngrok-operator-crds $(CRD_CHART_DIR) --install \
		--kube-context=kind-$(KIND_CLUSTER_NAME) \
	    --namespace kube-system \
		--create-namespace

	helm upgrade ngrok-operator-a $(HELM_CHART_DIR) --install \
		--kube-context=kind-$(KIND_CLUSTER_NAME) \
		--namespace namespace-a \
		--create-namespace \
		--set installCRDs=false \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set features.ingress.controllerName="k8s.ngrok.com/ingress-controller-a" \
		--set features.ingress.ingressClass.name="ngrok-a" \
		--set features.ingress.watchNamespace="namespace-a" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set ngrok.log.format=console \
		--set-string ngrok.log.level="8" \
		--set ngrok.log.stacktraceLevel=panic \
		--set ngrok.metadata.env=local,ngrok.metadata.from=makefile \
		$(HELM_DESCRIPTION_FLAG)

	helm upgrade ngrok-operator-b $(HELM_CHART_DIR) --install \
		--kube-context=kind-$(KIND_CLUSTER_NAME) \
		--namespace namespace-b \
		--create-namespace \
		--set installCRDs=false \
		--set features.ingress.controllerName="k8s.ngrok.com/ingress-controller-b" \
		--set features.ingress.ingressClass.name="ngrok-b" \
		--set features.ingress.watchNamespace="namespace-b" \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set ngrok.log.format=console \
		--set-string ngrok.log.level="8" \
		--set ngrok.log.stacktraceLevel=panic \
		--set ngrok.metadata.env=local,ngrok.metadata.from=makefile \
		$(HELM_DESCRIPTION_FLAG)

.PHONY: kind-load-image
kind-load-image: ## Load the locally built image into the kind cluster.
	kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME)

.PHONY: _deploy-check-env-vars
_deploy-check-env-vars:
ifndef NGROK_API_KEY
	$(error An NGROK_API_KEY must be set)
endif
ifndef NGROK_AUTHTOKEN
	$(error An NGROK_AUTHTOKEN must be set)
endif


.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	helm uninstall ngrok-operator --namespace $(KUBE_NAMESPACE)
