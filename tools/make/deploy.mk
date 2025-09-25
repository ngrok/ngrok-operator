##@ Deploying

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/api/main.go


.PHONY: deploy
deploy: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile &&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)
	kubectl rollout restart deployment $(KUBE_AGENT_MANAGER_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)

.PHONY: deploy_gateway
deploy_gateway: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile \
		--set useExperimentalGatewayApi=true &&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)
	kubectl rollout restart deployment $(KUBE_AGENT_MANAGER_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)

.PHONY: deploy_with_bindings
deploy_with_bindings: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"local\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile \
		--set bindings.enabled=true \
		&&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)
	kubectl rollout restart deployment $(KUBE_AGENT_MANAGER_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)

.PHONY: deploy_for_e2e
deploy_for_e2e: _deploy-check-env-vars docker-build manifests kustomize _helm_setup ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	helm upgrade $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --install \
		--namespace $(KUBE_NAMESPACE) \
		--create-namespace \
		--set oneClickDemoMode=$(DEPLOY_ONE_CLICK_DEMO_MODE) \
		--set image.repository=$(IMG) \
		--set image.tag="latest" \
		--set image.pullPolicy="Never" \
		--set podAnnotations."k8s\.ngrok\.com/test"="\{\"env\": \"e2e\"\}" \
		--set credentials.apiKey=$(NGROK_API_KEY) \
		--set credentials.authtoken=$(NGROK_AUTHTOKEN) \
		--set log.format=console \
		--set log.level=debug \
		--set log.stacktraceLevel=panic \
		--set metaData.env=local,metaData.from=makefile \
		--set bindings.enabled=true \
		--set bindings.serviceAnnotations.annotation1="val1" \
		--set bindings.serviceAnnotations.annotation2="val2" \
		--set bindings.serviceLabels.label1="val1" \
		&&\
	kubectl rollout restart deployment $(KUBE_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)
	kubectl rollout restart deployment $(KUBE_AGENT_MANAGER_DEPLOYMENT_NAME) -n $(KUBE_NAMESPACE)


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
	helm uninstall ngrok-operator
