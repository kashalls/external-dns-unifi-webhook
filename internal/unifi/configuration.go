package unifi

// Configuration holds configuration from environmental variables
type Configuration struct {
	Host          string `env:"UNIFI_HOST,notEmpty"`
	User          string `env:"UNIFI_USER,notEmpty"`
	Password      string `env:"UNIFI_PASS,notEmpty"`
	SkipTLSVerify bool   `env:"UNIFI_SKIP_TLS_VERIFY" envDefault:"false"`
}
