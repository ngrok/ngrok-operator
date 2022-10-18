#!/bin/bash

NGROK_LOG="${NGROK_LOG:-stdout}"
NGROK_REGION="${NGROK_REGION:-us}"
NGROK_REMOTE_MANAGEMENT="${NGROK_REMOTE_MANAGEMENT:-true}"

cat > /var/lib/ngrok/agent-template.yaml <<EOF
version: 2
authtoken: $NGROK_AUTHTOKEN
console_ui: false
log: $NGROK_LOG
region: $NGROK_REGION
remote_management: $NGROK_REMOTE_MANAGEMENT
update_check: false
EOF

ngrok start --none --config /var/lib/ngrok/agent-template.yaml
