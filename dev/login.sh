#!/bin/sh
# usage: ./login.sh <user> <password>
# Logs a test person in through Keycloak with curl and prints the identity the verify route received.
set -e
cd "$(dirname "$0")"
CJ=$(mktemp); CA=certs/ca.pem
page=$(curl -s -L --max-redirs 8 -c $CJ -b $CJ --cacert $CA https://verify.localhost.pomerium.io/)
action=$(printf '%s' "$page" | grep -o 'action="[^"]*"' | head -1 | sed 's/action="//;s/"$//;s/&amp;/\&/g')
[ -n "$action" ] || { echo "no login form"; printf '%s' "$page" | head -c 400; exit 1; }
final=$(curl -s -L --max-redirs 10 -c $CJ -b $CJ --cacert $CA -o /dev/null -w '%{url_effective} %{http_code}' \
  -X POST "$action" --data-urlencode "username=$1" --data-urlencode "password=$2" -d credentialId=)
case "$final" in *verify.localhost.pomerium.io*) ;; *) echo "login did not complete: $final"; exit 1;; esac
curl -s -L -c $CJ -b $CJ --cacert $CA https://verify.localhost.pomerium.io/json | python3 -I -c '
import json,sys
d=json.load(sys.stdin)
i=d.get("identity") or {}
print(json.dumps({k:i.get(k) for k in ("iss","aud","sub","email","name","groups")}))'
