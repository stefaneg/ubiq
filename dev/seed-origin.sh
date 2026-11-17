#!/bin/sh
# usage: ./seed-origin.sh [ref]
# Makes data/origin.git, the dev stack's origin: a bare repository whose main is this checkout's ref (HEAD by
# default). Leaves an existing origin alone; remove data/ and run `docker compose down -v` to start over.
set -eu
cd "$(dirname "$0")"
if [ -d data/origin.git ]; then
  echo "data/origin.git exists; left as it is"
  exit 0
fi
origin=$(pwd)/data/origin.git # absolute: git resolves a relative push URL from the checkout's top
git init -q --bare -b main "$origin"
git push -q "$origin" "${1:-HEAD}:refs/heads/main"
echo "wrote data/origin.git with main at ${1:-HEAD}"
