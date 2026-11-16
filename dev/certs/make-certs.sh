#!/bin/sh
# Makes a throwaway CA and a wildcard certificate for *.localhost.pomerium.io,
# which public DNS resolves to 127.0.0.1. RSA, because macOS LibreSSL writes EC keys Go rejects. Output stays untracked (see .gitignore).
set -eu
cd "$(dirname "$0")"

[ -f ca.pem ] || {
  openssl req -x509 -newkey rsa:2048 -nodes \
    -days 3650 -subj "/CN=ubiq dev CA" -keyout ca-key.pem -out ca.pem
}

openssl req -newkey rsa:2048 -nodes \
  -subj "/CN=localhost.pomerium.io" -keyout localhost.pomerium.io-key.pem -out server.csr
printf 'subjectAltName=DNS:localhost.pomerium.io,DNS:*.localhost.pomerium.io\n' > san.ext
openssl x509 -req -in server.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial \
  -days 825 -extfile san.ext -out localhost.pomerium.io.pem
rm -f server.csr san.ext ca.srl
chmod 644 localhost.pomerium.io-key.pem ca-key.pem
echo "wrote ca.pem and localhost.pomerium.io.pem"
