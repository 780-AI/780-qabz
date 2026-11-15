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

// debugSection is the top-level config key of the development settings (debug.*). Parse removes
// it in every mode other than ModeDev (task 020c).
const debugSection = "debug"

// ProvidersMinLoadInterval is the lowest allowed providers.general.load_interval (task 020). It
// equals provider.MinLoadInterval (a test in app checks it).
const ProvidersMinLoadInterval = time.Second

// The values of the return_type key of a provider mock (debug.mocks.<provider>.return_type, task 021).
const (
	MockReturnSuccess = "success"
	MockReturnError   = "error"
)

// ValidMockReturnType reports whether v is a value of a provider mock return_type.
func ValidMockReturnType(v string) bool { return v == MockReturnSuccess || v == MockReturnError }

// The values of the pay_return_type key of a provider mock (debug.mocks.<provider>.pay_return_type,
// task 044): MockReturnSuccess, or one of these.
const (
	MockReturnFailed  = "failed"  // the provider answers the payment as failed
	MockReturnPending = "pending" // the answer of a timeout: no final status
)

// ValidMockPayReturnType reports whether v is a value of a provider mock pay_return_type.
func ValidMockPayReturnType(v string) bool {
	return v == MockReturnSuccess || v == MockReturnFailed || v == MockReturnPending
}
