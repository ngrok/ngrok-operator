#!/bin/bash
set -ex

echo "Updating /etc/bash.bashrc and /etc/zsh/zshrc..."
if [[ -f /etc/bash.bashrc ]]; then
    echo -e 'eval "$(direnv hook bash)"' >> /etc/bash.bashrc
fi
if [ -f "/etc/zsh/zshrc" ]; then
    echo -e 'eval "$(direnv hook zsh)"' >> /etc/zsh/zshrc
fi