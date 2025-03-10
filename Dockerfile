# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.23 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go \
	go mod download

# Copy the go source
COPY . .

ARG TARGETOS TARGETARCH

# Build
RUN --mount=type=cache,target=/go \
	CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" make _build

# Prepare CA certificates
FROM golang:1.23 AS certs
WORKDIR /certs

# Copy the Intermediate CA Certificate directly into the image
COPY <<EOF ngrok-ca.crt
-----BEGIN CERTIFICATE-----
MIIDwjCCAqqgAwIBAgIUZqF2AkB17pISojTndgc2U5BDt7wwDQYJKoZIhvcNAQEL
BQAwbzEQMA4GA1UEAwwHUm9vdCBDQTENMAsGA1UECwwEcHJvZDESMBAGA1UECgwJ
bmdyb2sgSW5jMRYwFAYDVQQHDA1TYW4gRnJhbmNpc2NvMRMwEQYDVQQIDApDYWxp
Zm9ybmlhMQswCQYDVQQGEwJVUzAeFw0yMjA4MzExNDU5NDhaFw0zMjA4MjgxNDU5
NDhaMF8xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRIwEAYDVQQK
DAluZ3JvayBJbmMxDTALBgNVBAsMBHByb2QxGDAWBgNVBAMMD0ludGVybWVkaWF0
ZSBDQTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAK+t8q9Ost9BxCWX
fyGG0mVQOpIiyrzzZWyqT6CZpMY2fpOadLuZeBP7ti2Iw4FgCpfLntL0RldvMMNY
4qq61dVrCwhL/v2ldsaHUdzjtFj1i+ZNGUtV4E9korHxm2YdsD91w6WIjF/J0lvo
X2koLwFlGc/CkhT8z2VWebY8a6mYNyz5S7yPTQh2/mQ14lx/QPJgZSFEE/EEkMDC
bs4BoMuqKMhCpqEP8m4+CxPQ5/V6POSqUIxT4A7eWWj2MRpnmirmVbXOc24Aznqk
bdQUP4qagiR/i7qPsRx+f4mFfDninPsXp/djjByo0xzdh+i1HFyOR/7nyNDKlJ+e
rymRgnUCAwEAAaNmMGQwHQYDVR0OBBYEFJ47nRzHaOT+vY44N3TCMYtGlBjIMB8G
A1UdIwQYMBaAFNxeUxPIM8G7cX0DhFc81pLD4W+HMBIGA1UdEwEB/wQIMAYBAf8C
AQAwDgYDVR0PAQH/BAQDAgGGMA0GCSqGSIb3DQEBCwUAA4IBAQBRmnMoOtQbYL7P
Co1B5Chslb86HP2WI1jGRXhbfwAF2ySDFnX2ZbRPVtoQ+IuqXWxyXAeicYjXR6kz
xX8hLWfD14kWUIz6ZgT3uZrDSIzmQ+tz8ztbT6mTI1ECWdjLV/i58f6vKzgLD8Vp
3VdVns8NA9ee6a65QNjZEnwBVeccysoWkOwM/KzuazhSGcGu44y/S4ny9pAg7Pol
2kV4NicDKD6tSAdXmPmjFalYUfnMmyhurZIPrS2dgYgpOrGVMwronTOZ3BUf4DL4
zkkmcLXss1KztQnLd23nuNiIscwMcGM58a3O5zUp7aorfrm7cdRgkFmcYVNO/6uG
Q5iJ+Ppk
-----END CERTIFICATE-----
EOF
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
# Copy compiled binaries
COPY --from=builder /workspace/bin/api-manager /workspace/bin/agent-manager /workspace/bin/bindings-forwarder-manager ./
# Copy the intermediate CA into the system certificates directory
COPY --from=certs /certs/ngrok-ca.crt /etc/ssl/certs/ca-certificates.crt

USER 65532:65532

ENTRYPOINT ["/api-manager"]
