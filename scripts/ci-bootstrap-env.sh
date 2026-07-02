#!/usr/bin/env bash
# CI: synthesize a non-secret .env from the committed template.
set -euo pipefail

cp .env.example .env
sed -i 's/^CURATOR_API_TOKEN=change-me/CURATOR_API_TOKEN=dev-insecure-curator-token/' .env
