# Security Research Canary

This file was placed here by security researcher willardjansen as part of
responsible disclosure testing. If this file appears in the ngrok-operator
repository, it means the `generate-chart-readme.yaml` workflow checked out
fork code in a privileged `pull_request_target` context.

**This is a security vulnerability (CWE-78, CWE-829).**

The `pull_request_target` trigger combined with `actions/checkout` of the
PR author's fork code allows arbitrary code execution with `contents: write`
permission and access to repository secrets.

Contact: security@ngrok.com
Researcher: willardjansen
Date: 2026-03-30
