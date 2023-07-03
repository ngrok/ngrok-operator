#!/usr/bin/env bash

kind delete cluster
docker stop kind-registry | xargs docker rm