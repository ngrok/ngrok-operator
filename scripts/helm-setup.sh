#!/usr/bin/env bash

# This script is used to setup the environment for the project.

set -e

function cleanup() {
    rm -f HELM_PLUGIN_INSTALL.log
}

trap cleanup EXIT


function uninstall_helm_plugin() {
    local plugin_name=$1
    helm plugin uninstall $plugin_name
}

# Installs a plugin for helm if it is not already installed.
function helm_install_plugin() {
    local plugin_name=$1
    local plugin_source=$2
    local plugin_version=$3

    if ! helm plugin list | grep -q $plugin_name; then
        echo "Helm plugin $plugin_name not installed, installing..."
        helm plugin install $plugin_source --version $plugin_version >> HELM_PLUGIN_INSTALL.log 2>&1
        if [ $? -ne 0 ]; then
            echo "Failed to install helm plugin $plugin_name"
            cat HELM_PLUGIN_INSTALL.log
            exit 1
        fi
    else
        ## Check if version is correct
        if ! helm plugin list | grep -q $plugin_version; then
            echo "Helm plugin $plugin_name installed but wrong version. Uninstalling and installing desired version..."
            uninstall_helm_plugin $plugin_name

            helm plugin install $plugin_source --version $plugin_version >> HELM_PLUGIN_INSTALL.log 2>&1
            if [ $? -ne 0 ]; then
                echo "Failed to install helm plugin $plugin_name"
                cat HELM_PLUGIN_INSTALL.log
                exit 1
            fi
        else
            echo "Helm plugin $plugin_name already installed and up to date"
        fi
    fi
}

helm_install_plugin "unittest" "https://github.com/helm-unittest/helm-unittest" "0.6.1"
