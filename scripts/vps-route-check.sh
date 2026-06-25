#!/usr/bin/env bash
curl -sS -w " public_status:%{http_code}\n" http://127.0.0.1:8090/v1/public/status | head -c 300
echo
curl -sS -w " admin:%{http_code}\n" http://127.0.0.1:8090/v1/admin/pulse/health | head -c 300
echo
