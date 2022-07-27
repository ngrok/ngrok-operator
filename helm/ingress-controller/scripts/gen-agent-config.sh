#!/bin/bash

ls -altr
touch tmp
touch /tmp/tmp

NGORK_LOG="${NGROK_LOG:-stdout}"
NGROK_METADATA="${NGROK_METADATA:-{}}"
NGROK_REGION="${NGROK_REGION:-us}"
NGROK_REMOTE_MANAGEMENT="${NGROK_REMOTE_MANAGEMENT:-true}"

cat > /var/lib/ngrok/agent-template.yaml <<EOF
version: 2
authtoken: $NGROK_AUTHTOKEN
console_ui: false
log: $NGORK_LOG
metadata: $NGROK_METADATA
region: $NGROK_REGION
remote_management: $NGROK_REMOTE_MANAGEMENT
update_check: false
EOF

ngrok start --none --authtoken $NGROK_AUTHTOKEN
