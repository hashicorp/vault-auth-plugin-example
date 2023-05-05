#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


set -ex

GOOS=linux go build -o vault-auth-plugin-example cmd/vault-auth-plugin-example/main.go

docker kill vaultplg 2>/dev/null || true
tmpdir=$(mktemp -d vaultplgXXXXXX)
mkdir "$tmpdir/data"
docker pull hashicorp/vault
docker run --rm -d -p8200:8200 --name vaultplg -v "$(pwd)/$tmpdir/data":/data -v $(pwd):/example --cap-add=IPC_LOCK -e 'VAULT_LOCAL_CONFIG=
{
  "backend": {"file": {"path": "/data"}},
  "listener": [{"tcp": {"address": "0.0.0.0:8200", "tls_disable": true}}],
  "plugin_directory": "/example",
  "log_level": "debug",
  "disable_mlock": true,
  "api_addr": "http://localhost:8200"
}
' hashicorp/vault server
sleep 1

export VAULT_ADDR=http://localhost:8200

initoutput=$(vault operator init -key-shares=1 -key-threshold=1 -format=json)
vault operator unseal $(echo "$initoutput" | jq -r .unseal_keys_hex[0])

export VAULT_TOKEN=$(echo "$initoutput" | jq -r .root_token)

vault write sys/plugins/catalog/auth/example-auth-plugin \
    sha_256=$(shasum -a 256 vault-auth-plugin-example | cut -d' ' -f1) \
    command="vault-auth-plugin-example"

vault auth enable \
    -path="example" \
    -plugin-name="example-auth-plugin" \
    -plugin-version=0.2.0 \
    plugin

vault read -field=plugin_version sys/auth/example/tune

VAULT_TOKEN=  vault write auth/example/login password="super-secret-password"