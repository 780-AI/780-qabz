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
