#!/bin/sh
# Clones the dev origin into the Server's root when the volume holds no clone yet, then runs ubiq with the
# container's command. The Server reuses a clone it finds.
set -eu
[ -d /data/repo/.git ] || git clone -q /origin.git /data/repo
exec ubiq "$@"
