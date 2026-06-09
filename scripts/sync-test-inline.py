import json
import os
import sys
import urllib.request

analytics = os.environ.get("ANALYTICS_URL", "http://analytics:8080")
stream_id = os.environ.get("STREAM_ID", "318832886110")
login = os.environ.get("CHANNEL", "jynxzi")
url = f"{analytics}/v1/analytics/streams/{stream_id}/sync?channel={login}&viewers_only=true"
req = urllib.request.Request(url, method="POST")
try:
    with urllib.request.urlopen(req, timeout=300) as r:
        body = r.read().decode()
        print("status", r.status)
        print(body)
        d = json.loads(body)
        sys.exit(0 if d.get("success") else 1)
except urllib.error.HTTPError as e:
    print("http_error", e.code, e.read().decode())
    sys.exit(1)
