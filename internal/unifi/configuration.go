package unifi

// Configuration holds configuration from environmental variables
type Configuration struct {
	Host          string `env:"UNIFI_HOST,required"`
	User          string `env:"UNIFI_USER,required"`
	Password      string `env:"UNIFI_PASS,required"`
	SkipTLSVerify bool   `env:"UNIFI_SKIP_TLS_VERIFY" envDefault:"false"`
}
