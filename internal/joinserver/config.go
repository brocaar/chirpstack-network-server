package joinserver

// Config holds the join-server configuration.
type Config struct {
	Pool                Pool
	ResolveJoinEUI      bool   `mapstructure:"resolve_join_eui"`
	ResolveDomainSuffix string `mapstructure:"resolve_domain_suffix"`

	Certificates []struct {
		JoinEUI string `mapstructure:"join_eui"`
		CaCert  string `mapstructure:"ca_cert"`
		TLSCert string `mapstructure:"tls_cert"`
		TLSKey  string `mapstructure:"tls_key"`
	} `mapstructure:"certificates"`

	Default struct {
		Server  string
		CACert  string `mapstructure:"ca_cert"`
		TLSCert string `mapstructure:"tls_cert"`
		TLSKey  string `mapstructure:"tls_key"`
	}

	KEK struct {
		Set []struct {
			Label string
			KEK   string `mapstructure:"kek"`
		}
	} `mapstructure:"kek"`
}
