#!/usr/bin/env bash
printf '%s\n' \
  'Hosted production ops are maintained in private streampulse-ops.' \
  'This public repo contains only local/self-hosted examples.' \
  'Use: make up, make smoke, make compose-config-check' \
  'Do not put production secrets or host-specific runbooks here.' >&2
exit 0
