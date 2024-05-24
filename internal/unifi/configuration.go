package unifi

// Configuration holds configuration from environmental variables
type Configuration struct {
	Host          string `env:"UNIFI_HOST"`
	User          string `env:"UNIFI_USER" envDefault:"external-dns-unifi"`
	Password      string `env:"UNIFI_PASS" envDefault:"V3ryS3cur3!!"`
	SkipTLSVerify bool   `env:"UNIFI_SKIP_TLS_VERIFY" envDefault:"false"`
}
