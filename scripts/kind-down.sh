#!/usr/bin/env bash

kind delete cluster --name ngrok-ingress-controller
docker stop kind-registry | xargs docker rm