# The plugin binary is built on the host (go build resolves the local replace directive),
# then packaged into a minimal image. The image is used as a kind init container that
# copies the binary to a shared emptyDir volume at startup.
#
# Build sequence: make plugin-deploy runs `go build` first, then this Dockerfile.
FROM alpine:3.21
COPY bin/ngrok-traffic-router /ngrok-traffic-router
RUN chmod +x /ngrok-traffic-router
# Default command: copy binary to the shared plugins volume and exit.
CMD ["cp", "/ngrok-traffic-router", "/plugins/ngrok-traffic-router"]
