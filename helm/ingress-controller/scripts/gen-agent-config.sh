#!/bin/bash

NGROK_REMOTE_MANAGEMENT="${NGROK_REMOTE_MANAGEMENT:-true}"

cat > /var/lib/ngrok/agent-template.yaml <<EOF
version: 2
authtoken: $NGROK_AUTHTOKEN
console_ui: false
region: $NGROK_REGION
remote_management: $NGROK_REMOTE_MANAGEMENT
update_check: false
EOF

ngrok start --none --config /var/lib/ngrok/agent-template.yaml
