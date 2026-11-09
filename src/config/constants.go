package config

import "time"

// CacheServicesTTL is how long GET /v1/services keeps the service list of one language in memory
// before it reads the services table again.
const CacheServicesTTL = 60 * time.Second

// The values of the required config key mode (Config.Mode).
const (
	ModeDev   = "dev"
	ModeStage = "stage"
	ModeProd  = "prod"
)

// ProvidersMinLoadInterval is the lowest allowed providers.general.load_interval (task 020). It
// equals provider.MinLoadInterval (a test in app checks it).
const ProvidersMinLoadInterval = time.Second
