#!/bin/bash

# Run from root project directory as `./scripts/init_e2e_configs.sh <subdomain>`

# Check if the domain argument is provided
if [ -z "$1" ]; then
    echo "Please provide the desired subdomain as an argument."
    exit 1
fi

prefix="EXAMPLE-"

domain="$1"

# Find all EXAMPLE config files in the e2e-fixtures directory and its subdirectories
find "./e2e-fixtures" -type f -name "$prefix*" -print0 | while IFS= read -r -d '' file; do
    directory="$(dirname "$file")"

    # Get the filename without the prefix
    filename="${file##*/}"
    target_filename="${filename#$prefix}"
    target_file="$directory/$target_filename"

    # Copy the file over, sans prefix
    cp "$file" "$target_file"

    # Replace text within the copied file
    sed -i "s/<UNIQUE SUBDOMAIN>/$domain/g" "$target_file"
    sed -i "s/<UNIQUE SUBDOMAIN 2>/$domain/g" "$target_file"
done
