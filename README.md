# Sample Auth Method Plugin for HashiCorp Vault

This repository contains sample code for a HashiCorp Vault Auth Plugin. It is
both a real custom Vault auth method, and an example of how to build, install,
and maintain your own Vault auth plugin.

**This code is for educational purposes only. It demonstrates a basic Vault
Auth Plugin. It is not secure. Do not use it in production.**

For more information, see the [accompanying blog post](https://www.hashicorp.com/blog/building-a-vault-secure-plugin).

## Setup

The setup guide assumes some familiarity with Vault and Vault's plugin
ecosystem. You must have a Vault server already running, unsealed, and
authenticated.

1. Download and decompress the latest plugin binary from the Releases tab on
GitHub. Alternatively you can compile the plugin from source.

1. Move the compiled plugin into Vault's configured `plugin_directory`:

    ```sh
    $ mv vault-auth-plugin-example /etc/vault/plugins/vault-auth-plugin-example
    ```

1. Calculate the SHA256 of the plugin and register it in Vault's plugin catalog.
If you are downloading the pre-compiled binary, it is highly recommended that
you use the published checksums to verify integrity.

    ```sh
    $ export SHA256=$(shasum -a 256 "/etc/vault/plugins/vault-auth-plugin-example" | cut -d' ' -f1)

    $ vault plugin register \
        -sha256="${SHA256}" \
        -command="vault-auth-plugin-example" \
        auth example-auth-plugin
    ```

1. Mount the auth method:

    ```sh
    $ vault auth enable \
        -path="example" \
        -plugin-name="example-auth-plugin" plugin
    ```

## Authenticating with the Shared Secret

To authenticate, the user supplies the shared secret:

```sh
$ vault write auth/example/login password="super-secret-password"
```

The response will be a standard auth response with some token metadata:

```text
Key             	Value
---             	-----
token           	b62420a6-ee83-22a4-7a15-a908af658c9f
token_accessor  	9eff2c4e-e321-3903-413e-a5084abb631e
token_duration  	30s
token_renewable 	true
token_policies  	[default my-policy other-policy]
token_meta_fruit	"banana"
```

## Should I Use This?

No, please do not. This is an example Vault Plugin that should be use for
learning purposes. Having a shared phrase that gives anyone access to Vault is
highly discouraged and a security anti-pattern. This code should be used for
educational purposes only.

## License

This code is licensed under the MPLv2 license.
