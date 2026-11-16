#!/bin/sh
# usage: ./login.sh <user> <password>
# Plays a browser with curl: opens the page route, logs a test person in at Keycloak's form, and prints the Person
# and role the page shows, e.g. {"name":"Wren Writer","role":"Writer"}.
set -e
cd "$(dirname "$0")"
CJ=$(mktemp); PAGE=$(mktemp); CA=certs/ca.pem; URL=https://ubiq.localhost.pomerium.io/
form=$(curl -s -L --max-redirs 8 -c "$CJ" -b "$CJ" --cacert $CA $URL)
action=$(printf '%s' "$form" | grep -o 'action="[^"]*"' | head -1 | sed 's/action="//;s/"$//;s/&amp;/\&/g')
[ -n "$action" ] || { echo "no login form"; printf '%s' "$form" | head -c 400; exit 1; }
final=$(curl -s -L --max-redirs 10 -c "$CJ" -b "$CJ" --cacert $CA -o "$PAGE" -w '%{url_effective} %{http_code}' \
  "$action" --data-urlencode "username=$1" --data-urlencode "password=$2" -d credentialId=)
case "$final" in *ubiq.localhost.pomerium.io*200) ;; *) echo "login did not reach the page: $final"; exit 1;; esac
grep -o '"person":{[^}]*}' "$PAGE" | sed 's/^"person"://' || { echo "the page shows no Person"; exit 1; }
