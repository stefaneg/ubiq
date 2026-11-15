#!/bin/sh
# usage: ./mcp-call.sh <token-file> <tool> [json-arguments]
# Calls one MCP tool over the MCP route with the bearer token mcp-login.sh saved, and prints the result text.
# Example: ./mcp-call.sh /tmp/writer.tok define_term '{"name":"Batch Window","definition":"..."}'
set -e
cd "$(dirname "$0")"
CA=certs/ca.pem; AS=https://mcp.localhost.pomerium.io
at=$(cat "$1"); tool=$2; args=${3:-'{}'}
curl -s --cacert $CA -H "Authorization: Bearer $at" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  $AS/mcp -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$tool\",\"arguments\":$args}}" \
  | sed -n 's/^data: //p' | python3 -I -c '
import sys, json
for line in sys.stdin:
    d = json.loads(line)
    if "error" in d:
        print("PROTOCOL ERROR:", d["error"]); continue
    r = d["result"]
    if r.get("isError"): print("REFUSED:")
    for c in r.get("content", []):
        print(c.get("text", c))'
