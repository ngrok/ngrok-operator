# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.3.0
### Changed

- Moved from calling ngrok-agent sidecar to using the ngrok-go library in the controller process.
- Moved `apiKey` and `authtoken` to `credentials.apiKey` and `credentials.authtoken` respectively.
- `credentialSecrets.name` is now `credentials.secret.name`
- Changed replicas to 1 by default to work better for default/demo setup.

## 0.2.0
### Added

- Support for different values commonly found in helm charts

# 0.1.0

TODO
