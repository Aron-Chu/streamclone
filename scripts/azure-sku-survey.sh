#!/usr/bin/env bash
set -euo pipefail

regions=(westus2 westus3 centralus eastus2 southcentralus eastus)
skus=(Standard_B2as_v2 Standard_B2s Standard_B1s Standard_B2ms Standard_D2s_v3 Standard_D2s_v4 Standard_D2as_v4 Standard_A2_v2)

for r in "${regions[@]}"; do
  echo "=== $r ==="
  for s in "${skus[@]}"; do
    st=$(az vm list-skus -l "$r" --size "$s" -o json 2>/dev/null | python3 -c "
import json, sys
d = json.load(sys.stdin)
if not d:
    print('NOT_LISTED')
elif not d[0].get('restrictions'):
    print('OK')
elif any(x.get('type') == 'Location' for x in d[0]['restrictions']):
    print('BLOCKED')
else:
    print('NONZONAL_OK')
" 2>/dev/null || echo ERROR)
    printf "  %-22s %s\n" "$s" "$st"
  done
  echo
done
