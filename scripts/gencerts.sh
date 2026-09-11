#!/usr/bin/env bash
# Generate independent inner/outer CAs and separate Gate/Voyager and client secrets.
set -euo pipefail
umask 077
dest=${1:?Usage: bash scripts/gencerts.sh /absolute/path/to/new-test-credentials}
days=${2:-2}
if ! [[ "$days" =~ ^[0-9]+$ ]] || ((days < 1 || days > 365)); then
  echo 'Certificate lifetime must be 1..365 days' >&2
  exit 1
fi
mkdir "$dest"
cd "$dest"
cat > ca.cnf <<'EOF'
[req]
distinguished_name=dn
x509_extensions=ca_ext
prompt=no
[dn]
CN=Local test CA
[ca_ext]
basicConstraints=critical,CA:TRUE
keyUsage=critical,keyCertSign,cRLSign
EOF
for layer in inner outer; do
  openssl req -x509 -sha256 -config ca.cnf -newkey ec -pkeyopt ec_paramgen_curve:P-256 -pkeyopt ec_param_enc:named_curve -nodes \
    -keyout "$layer-ca.key" -out "$layer-ca.pem" -days "$days" \
    -subj "/CN=Local test $layer CA" >/dev/null 2>&1
  openssl req -new -sha256 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -pkeyopt ec_param_enc:named_curve -nodes \
    -keyout "$layer.key" -out "$layer.csr" \
    -subj "/CN=Local test $layer server" >/dev/null 2>&1
done
cat > inner.ext <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth
subjectAltName=DNS:voyager.internal
EOF
cat > outer.ext <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth
subjectAltName=DNS:localhost,DNS:voyager,IP:127.0.0.1
EOF
for layer in inner outer; do
  openssl x509 -req -sha256 -in "$layer.csr" -CA "$layer-ca.pem" -CAkey "$layer-ca.key" \
    -CAcreateserial -out "$layer.pem" -days "$days" -extfile "$layer.ext" >/dev/null 2>&1
done
openssl rand -hex 32 > token.txt
openssl rand -hex 32 > client-password.txt
printf 'Created two independent CAs and separate Gate/Voyager and client secrets in %s\n' "$dest"
