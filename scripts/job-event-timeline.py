#!/usr/bin/env python3
import json
import sys
import urllib.request

job_id = sys.argv[1] if len(sys.argv) > 1 else "3262b0ae038f47908b73c8f5941a5c3e"
with urllib.request.urlopen(f"http://127.0.0.1:8095/v1/jobs/{job_id}", timeout=10) as r:
    data = json.loads(r.read().decode())
events = data["events"]
prev = None
print(f"Job {job_id} state={data['job']['state']}")
for e in events:
    t = e["created_at"]
    delta = (t - prev) if prev is not None else 0
    prev = t
    print(f"  {e['state']:20} +{delta/1000:6.1f}s  {e['message']}")
