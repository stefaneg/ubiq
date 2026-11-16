#!/bin/bash
# The instance's first boot, run once by cloud-init after it writes /etc/ubiq/provision.env from user-data
# (deploy/terraform): renders Pomerium's configuration and the Server's unit, generates Pomerium's secrets, writes the
# ubiq user's git credential, and starts both units. The Server clones origin by itself.
set -euo pipefail

env=/etc/ubiq/provision.env
# shellcheck source=/dev/null
source "$env"
: "${DOMAIN:?}" "${IDP_ISSUER:?}" "${IDP_CLIENT_ID:?}" "${IDP_CLIENT_SECRET:?}" "${WRITER_CLAIM:?}" "${WRITERS:?}"
: "${ORIGIN:?}" "${PUSH_ACCOUNT:?}" "${PUSH_TOKEN:?}" "${COMMITTER:?}"

# render prints the file $1 with each placeholder @NAME@ replaced by its value, given as NAME VALUE pairs.
render() {
	local text
	text=$(<"$1")
	shift
	while (($#)); do
		text=${text//"@$1@"/"$2"}
		shift 2
	done
	printf '%s\n' "$text"
}

# yaml escapes a value for a double-quoted YAML string.
yaml() {
	local v=${1//\\/\\\\}
	printf '%s' "${v//\"/\\\"}"
}

# unit escapes a value for a double-quoted word on a systemd ExecStart line.
unit() {
	local v=${1//\\/\\\\}
	v=${v//\"/\\\"}
	v=${v//%/%%}
	printf '%s' "${v//\$/\$\$}"
}

secret() {
	head -c 32 /dev/urandom | base64
}

umask 077
render /usr/local/share/ubiq/pomerium.yaml.tmpl \
	DOMAIN "$(yaml "$DOMAIN")" \
	SHARED_SECRET "$(secret)" \
	COOKIE_SECRET "$(secret)" \
	IDP_ISSUER "$(yaml "$IDP_ISSUER")" \
	IDP_CLIENT_ID "$(yaml "$IDP_CLIENT_ID")" \
	IDP_CLIENT_SECRET "$(yaml "$IDP_CLIENT_SECRET")" \
	>/etc/pomerium/config.yaml
chown pomerium:pomerium /etc/pomerium/config.yaml

umask 022
unit=/etc/systemd/system/ubiq.service
render "$unit" \
	ORIGIN "$(unit "$ORIGIN")" \
	DOMAIN "$(unit "$DOMAIN")" \
	WRITER_CLAIM "$(unit "$WRITER_CLAIM")" \
	WRITERS "$(unit "$WRITERS")" \
	COMMITTER "$(unit "$COMMITTER")" \
	>"$unit.new"
mv "$unit.new" "$unit"

# The push credential: git's own store, one line for origin's host, read by git on every push.
host=${ORIGIN#https://}
host=${host%%/*}
umask 077
printf 'https://%s:%s@%s\n' "$PUSH_ACCOUNT" "$PUSH_TOKEN" "$host" >/var/lib/ubiq/.git-credentials
chown ubiq:ubiq /var/lib/ubiq/.git-credentials

systemctl daemon-reload
systemctl enable --now pomerium.service ubiq.service
