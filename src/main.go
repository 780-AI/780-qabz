package main

import (
	"fmt"

	// Base packages. Blank imports keep them in go.mod until real code uses them.
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/prometheus/client_golang/prometheus"
	_ "github.com/redis/go-redis/v9"
	_ "go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	_ "go.opentelemetry.io/otel/sdk/trace"
	_ "go.uber.org/zap"
)

func main() {
	fmt.Println("hello world")
}
