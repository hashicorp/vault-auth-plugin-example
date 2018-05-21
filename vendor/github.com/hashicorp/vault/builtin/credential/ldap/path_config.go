package ldap

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"text/template"

	"github.com/hashicorp/errwrap"
	"github.com/hashicorp/vault/helper/consts"
	"github.com/hashicorp/vault/helper/ldaputil"
	"github.com/hashicorp/vault/helper/tlsutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathConfig(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: `config`,
		Fields: map[string]*framework.FieldSchema{
			"url": &framework.FieldSchema{
				Type:        framework.TypeString,
				Default:     "ldap://127.0.0.1",
				Description: "LDAP URL to connect to (default: ldap://127.0.0.1). Multiple URLs can be specified by concatenating them with commas; they will be tried in-order.",
			},

			"userdn": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "LDAP domain to use for users (eg: ou=People,dc=example,dc=org)",
			},

			"binddn": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "LDAP DN for searching for the user DN (optional)",
			},

			"bindpass": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "LDAP password for searching for the user DN (optional)",
			},

			"groupdn": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "LDAP search base to use for group membership search (eg: ou=Groups,dc=example,dc=org)",
			},

			"groupfilter": &framework.FieldSchema{
				Type:    framework.TypeString,
				Default: "(|(memberUid={{.Username}})(member={{.UserDN}})(uniqueMember={{.UserDN}}))",
				Description: `Go template for querying group membership of user (optional)
The template can access the following context variables: UserDN, Username
Example: (&(objectClass=group)(member:1.2.840.113556.1.4.1941:={{.UserDN}}))
Default: (|(memberUid={{.Username}})(member={{.UserDN}})(uniqueMember={{.UserDN}}))`,
			},

			"groupattr": &framework.FieldSchema{
				Type:    framework.TypeString,
				Default: "cn",
				Description: `LDAP attribute to follow on objects returned by <groupfilter>
in order to enumerate user group membership.
Examples: "cn" or "memberOf", etc.
Default: cn`,
			},

			"upndomain": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Enables userPrincipalDomain login with [username]@UPNDomain (optional)",
			},

			"userattr": &framework.FieldSchema{
				Type:        framework.TypeString,
				Default:     "cn",
				Description: "Attribute used for users (default: cn)",
			},

			"certificate": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "CA certificate to use when verifying LDAP server certificate, must be x509 PEM encoded (optional)",
			},

			"discoverdn": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Description: "Use anonymous bind to discover the bind DN of a user (optional)",
			},

			"insecure_tls": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Description: "Skip LDAP server SSL Certificate verification - VERY insecure (optional)",
			},

			"starttls": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Description: "Issue a StartTLS command after establishing unencrypted connection (optional)",
			},

			"tls_min_version": &framework.FieldSchema{
				Type:        framework.TypeString,
				Default:     "tls12",
				Description: "Minimum TLS version to use. Accepted values are 'tls10', 'tls11' or 'tls12'. Defaults to 'tls12'",
			},

			"tls_max_version": &framework.FieldSchema{
				Type:        framework.TypeString,
				Default:     "tls12",
				Description: "Maximum TLS version to use. Accepted values are 'tls10', 'tls11' or 'tls12'. Defaults to 'tls12'",
			},

			"deny_null_bind": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Default:     true,
				Description: "Denies an unauthenticated LDAP bind request if the user's password is empty; defaults to true",
			},

			"case_sensitive_names": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Description: "If true, case sensitivity will be used when comparing usernames and groups for matching policies.",
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ReadOperation:   b.pathConfigRead,
			logical.UpdateOperation: b.pathConfigWrite,
		},

		HelpSynopsis:    pathConfigHelpSyn,
		HelpDescription: pathConfigHelpDesc,
	}
}

/*
 * Construct ConfigEntry struct using stored configuration.
 */
func (b *backend) Config(ctx context.Context, req *logical.Request) (*ldaputil.ConfigEntry, error) {
	// Schema for ConfigEntry
	fd, err := b.getConfigFieldData()
	if err != nil {
		return nil, err
	}

	// Create a new ConfigEntry, filling in defaults where appropriate
	result, err := b.newConfigEntry(fd)
	if err != nil {
		return nil, err
	}

	storedConfig, err := req.Storage.Get(ctx, "config")
	if err != nil {
		return nil, err
	}

	if storedConfig == nil {
		// No user overrides, return default configuration
		result.CaseSensitiveNames = new(bool)
		*result.CaseSensitiveNames = false

		return result, nil
	}

	// Deserialize stored configuration.
	// Fields not specified in storedConfig will retain their defaults.
	if err := storedConfig.DecodeJSON(&result); err != nil {
		return nil, err
	}

	var persistNeeded bool
	if result.CaseSensitiveNames == nil {
		// Upgrade from before switching to case-insensitive
		result.CaseSensitiveNames = new(bool)
		*result.CaseSensitiveNames = true
		persistNeeded = true
	}

	if persistNeeded && (b.System().LocalMount() || !b.System().ReplicationState().HasState(consts.ReplicationPerformanceSecondary)) {
		entry, err := logical.StorageEntryJSON("config", result)
		if err != nil {
			return nil, err
		}
		if err := req.Storage.Put(ctx, entry); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (b *backend) pathConfigRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	cfg, err := b.Config(ctx, req)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	resp := &logical.Response{
		Data: map[string]interface{}{
			"url":                  cfg.Url,
			"userdn":               cfg.UserDN,
			"groupdn":              cfg.GroupDN,
			"groupfilter":          cfg.GroupFilter,
			"groupattr":            cfg.GroupAttr,
			"upndomain":            cfg.UPNDomain,
			"userattr":             cfg.UserAttr,
			"certificate":          cfg.Certificate,
			"insecure_tls":         cfg.InsecureTLS,
			"starttls":             cfg.StartTLS,
			"binddn":               cfg.BindDN,
			"deny_null_bind":       cfg.DenyNullBind,
			"discoverdn":           cfg.DiscoverDN,
			"tls_min_version":      cfg.TLSMinVersion,
			"tls_max_version":      cfg.TLSMaxVersion,
			"case_sensitive_names": *cfg.CaseSensitiveNames,
		},
	}
	return resp, nil
}

/*
 * Creates and initializes a ConfigEntry object with its default values,
 * as specified by the passed schema.
 */
func (b *backend) newConfigEntry(d *framework.FieldData) (*ldaputil.ConfigEntry, error) {
	cfg := new(ldaputil.ConfigEntry)

	url := d.Get("url").(string)
	if url != "" {
		cfg.Url = strings.ToLower(url)
	}
	userattr := d.Get("userattr").(string)
	if userattr != "" {
		cfg.UserAttr = strings.ToLower(userattr)
	}
	userdn := d.Get("userdn").(string)
	if userdn != "" {
		cfg.UserDN = userdn
	}
	groupdn := d.Get("groupdn").(string)
	if groupdn != "" {
		cfg.GroupDN = groupdn
	}
	groupfilter := d.Get("groupfilter").(string)
	if groupfilter != "" {
		// Validate the template before proceeding
		_, err := template.New("queryTemplate").Parse(groupfilter)
		if err != nil {
			return nil, errwrap.Wrapf("invalid groupfilter: {{err}}", err)
		}

		cfg.GroupFilter = groupfilter
	}
	groupattr := d.Get("groupattr").(string)
	if groupattr != "" {
		cfg.GroupAttr = groupattr
	}
	upndomain := d.Get("upndomain").(string)
	if upndomain != "" {
		cfg.UPNDomain = upndomain
	}
	certificate := d.Get("certificate").(string)
	if certificate != "" {
		block, _ := pem.Decode([]byte(certificate))

		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("failed to decode PEM block in the certificate")
		}
		_, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errwrap.Wrapf("failed to parse certificate: {{err}}", err)
		}
		cfg.Certificate = certificate
	}
	insecureTLS := d.Get("insecure_tls").(bool)
	if insecureTLS {
		cfg.InsecureTLS = insecureTLS
	}
	cfg.TLSMinVersion = d.Get("tls_min_version").(string)
	if cfg.TLSMinVersion == "" {
		return nil, fmt.Errorf("failed to get 'tls_min_version' value")
	}

	var ok bool
	_, ok = tlsutil.TLSLookup[cfg.TLSMinVersion]
	if !ok {
		return nil, fmt.Errorf("invalid 'tls_min_version'")
	}

	cfg.TLSMaxVersion = d.Get("tls_max_version").(string)
	if cfg.TLSMaxVersion == "" {
		return nil, fmt.Errorf("failed to get 'tls_max_version' value")
	}

	_, ok = tlsutil.TLSLookup[cfg.TLSMaxVersion]
	if !ok {
		return nil, fmt.Errorf("invalid 'tls_max_version'")
	}
	if cfg.TLSMaxVersion < cfg.TLSMinVersion {
		return nil, fmt.Errorf("'tls_max_version' must be greater than or equal to 'tls_min_version'")
	}

	startTLS := d.Get("starttls").(bool)
	if startTLS {
		cfg.StartTLS = startTLS
	}

	bindDN := d.Get("binddn").(string)
	if bindDN != "" {
		cfg.BindDN = bindDN
	}

	bindPass := d.Get("bindpass").(string)
	if bindPass != "" {
		cfg.BindPassword = bindPass
	}

	denyNullBind := d.Get("deny_null_bind").(bool)
	if denyNullBind {
		cfg.DenyNullBind = denyNullBind
	}

	discoverDN := d.Get("discoverdn").(bool)
	if discoverDN {
		cfg.DiscoverDN = discoverDN
	}

	caseSensitiveNames, ok := d.GetOk("case_sensitive_names")
	if ok {
		cfg.CaseSensitiveNames = new(bool)
		*cfg.CaseSensitiveNames = caseSensitiveNames.(bool)
	}

	return cfg, nil
}

func (b *backend) pathConfigWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	// Build a ConfigEntry struct out of the supplied FieldData
	cfg, err := b.newConfigEntry(d)
	if err != nil {
		return logical.ErrorResponse(err.Error()), nil
	}

	// On write, if not specified, use false. We do this here so upgrade logic
	// works since it calls the same newConfigEntry function
	if cfg.CaseSensitiveNames == nil {
		cfg.CaseSensitiveNames = new(bool)
		*cfg.CaseSensitiveNames = false
	}

	entry, err := logical.StorageEntryJSON("config", cfg)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}

	return nil, nil
}

/*
 * Returns FieldData describing our ConfigEntry struct schema
 */
func (b *backend) getConfigFieldData() (*framework.FieldData, error) {
	configPath := b.Route("config")

	if configPath == nil {
		return nil, logical.ErrUnsupportedPath
	}

	raw := make(map[string]interface{}, len(configPath.Fields))

	fd := framework.FieldData{
		Raw:    raw,
		Schema: configPath.Fields,
	}

	return &fd, nil
}

const pathConfigHelpSyn = `
Configure the LDAP server to connect to, along with its options.
`

const pathConfigHelpDesc = `
This endpoint allows you to configure the LDAP server to connect to and its
configuration options.

The LDAP URL can use either the "ldap://" or "ldaps://" schema. In the former
case, an unencrypted connection will be made with a default port of 389, unless
the "starttls" parameter is set to true, in which case TLS will be used. In the
latter case, a SSL connection will be established with a default port of 636.

## A NOTE ON ESCAPING

It is up to the administrator to provide properly escaped DNs. This includes
the user DN, bind DN for search, and so on.

The only DN escaping performed by this backend is on usernames given at login
time when they are inserted into the final bind DN, and uses escaping rules
defined in RFC 4514.

Additionally, Active Directory has escaping rules that differ slightly from the
RFC; in particular it requires escaping of '#' regardless of position in the DN
(the RFC only requires it to be escaped when it is the first character), and
'=', which the RFC indicates can be escaped with a backslash, but does not
contain in its set of required escapes. If you are using Active Directory and
these appear in your usernames, please ensure that they are escaped, in
addition to being properly escaped in your configured DNs.

For reference, see https://www.ietf.org/rfc/rfc4514.txt and
http://social.technet.microsoft.com/wiki/contents/articles/5312.active-directory-characters-to-escape.aspx
`
