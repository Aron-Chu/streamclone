#!/usr/bin/env python3
import json
import subprocess
import sys
import urllib.error
import urllib.request


def main() -> int:
    names = subprocess.check_output(
        ["docker", "ps", "--format", "{{.Names}}"],
        text=True,
    ).splitlines()
    clipper = next((n for n in names if "clipper" in n), "")
    if not clipper:
        print(json.dumps({"error": "clipper_container_not_found", "containers": names[:10]}))
        return 1
    script = r"""
import json, os, urllib.request, urllib.error
tok=os.environ.get('CLIPPER_TWITCH_USER_ACCESS_TOKEN','')
cid=os.environ.get('CLIPPER_TWITCH_CLIENT_ID') or os.environ.get('TWITCH_OAUTH_CLIENT_ID') or ''
out={'container_token_len':len(tok),'container_token_prefix':tok[:4] if len(tok)>=4 else '','container_client_prefix':cid[:4] if len(cid)>=4 else ''}
if tok:
    req=urllib.request.Request('https://id.twitch.tv/oauth2/validate', headers={'Authorization':'Bearer '+tok.strip()})
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body=json.loads(resp.read().decode())
            out['validate_status']=resp.status
            out['has_clips_edit']='clips:edit' in (body.get('scopes') or [])
    except urllib.error.HTTPError as exc:
        out['validate_status']=exc.code
        out['validate_body']=exc.read().decode('utf-8','replace')[:160]
print(json.dumps(out))
"""
    out = subprocess.check_output(["docker", "exec", clipper, "python3", "-c", script], text=True)
    inspect = subprocess.check_output(
        ["docker", "inspect", clipper, "--format", "{{.Created}}|{{.State.StartedAt}}"],
        text=True,
    ).strip()
    created, started = inspect.split("|", 1)
    print(json.dumps({"container": clipper, "created": created, "started": started, **json.loads(out)}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
