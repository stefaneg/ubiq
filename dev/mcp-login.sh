#!/bin/sh
# usage: ./mcp-login.sh <user> <password> [token-file]
# Plays an MCP client against the MCP route with curl, as Claude Code connects: its client ID metadata document as
# client_id, the PKCE authorization-code flow through Keycloak's form, the code exchanged for a token, then whoami and
# lookup with the bearer token. Writes the access token to token-file when given. Nothing listens on the callback:
# curl's final effective URL carries the code, and a loopback redirect matches Claude Code's registered
# http://127.0.0.1/callback whatever the port.
set -e
cd "$(dirname "$0")"
CA=certs/ca.pem; AS=https://mcp.localhost.pomerium.io; CJ=$(mktemp)
CID=https://claude.ai/oauth/claude-code-client-metadata
RU=http://127.0.0.1:1234/callback
enc() { python3 -I -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$1"; }
echo "1 client $CID"
ver=$(python3 -I -c 'import secrets; print(secrets.token_urlsafe(48))')
chal=$(printf '%s' "$ver" | python3 -I -c 'import sys,hashlib,base64; print(base64.urlsafe_b64encode(hashlib.sha256(sys.stdin.read().encode()).digest()).rstrip(b"=").decode())')
url="$AS/.pomerium/mcp/authorize?client_id=$(enc $CID)&redirect_uri=$(enc $RU)&response_type=code&code_challenge=$chal&code_challenge_method=S256&state=xyz&resource=$(enc $AS/mcp)"
page=$(curl -s -L --max-redirs 8 -c "$CJ" -b "$CJ" --cacert $CA -w '\nHOPS=%{num_redirects} %{http_code}' "$url")
echo "2 authorize: $(printf '%s' "$page" | tail -1), login form shown"
action=$(printf '%s' "$page" | grep -o 'action="[^"]*"' | head -1 | sed 's/action="//;s/"$//;s/&amp;/\&/g')
[ -n "$action" ] || { echo "no login form"; printf '%s' "$page" | head -c 600; exit 1; }
final=$(curl -s -L --max-redirs 10 --connect-timeout 2 -c "$CJ" -b "$CJ" --cacert $CA -o /dev/null -w '%{url_effective} HOPS=%{num_redirects}' \
  "$action" --data-urlencode "username=$1" --data-urlencode "password=$2" -d credentialId= || true)
echo "3 login form: $(printf '%s' "$final" | sed 's/code=[^&]*/code=.../')"
code=$(printf '%s' "$final" | sed -n 's/.*[?&]code=\([^& ]*\).*/\1/p' | python3 -I -c 'import sys,urllib.parse; print(urllib.parse.unquote(sys.stdin.read().strip()))')
[ -n "$code" ] || { echo "no code"; exit 1; }
tok=$(curl -s --cacert $CA -X POST $AS/.pomerium/mcp/token -d grant_type=authorization_code --data-urlencode "code=$code" \
  --data-urlencode "redirect_uri=$RU" --data-urlencode "client_id=$CID" -d "code_verifier=$ver" --data-urlencode "resource=$AS/mcp")
echo "4 token: $(printf '%s' "$tok" | python3 -I -c 'import sys,json; d=json.load(sys.stdin); print({k:(v if k not in ("access_token","refresh_token") else v[:10]+"...") for k,v in d.items()})')"
at=$(printf '%s' "$tok" | python3 -I -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')
[ -z "$3" ] || printf '%s' "$at" > "$3"
printf '%s' "$at" > "$CJ.tok"
echo "5 whoami over MCP:"; ./mcp-call.sh "$CJ.tok" whoami
echo "6 lookup Term over MCP:"; ./mcp-call.sh "$CJ.tok" lookup '{"words":["Term"]}' | head -3
rm -f "$CJ.tok"
