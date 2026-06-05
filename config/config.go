package config

import (
	"os"
	"time"
)

// config struct
type Config struct {
	Port            string        `env:"PORT"`
	ProcessingDelay time.Duration `env:"PROCESSING_DELAY"`
	IdempotencyTTL  time.Duration `env:"IDEMPOTENCY_TTL"`
}

// config function to load config from env variables
func Load() *Config {
	port := os.Getenv("PORT") //get port from env file
	if port == "" {
		port = "8080"
	}
	return &Config{
		Port:            port,
		ProcessingDelay: 2 * time.Second, //delay or simulate processing(2sec)
		IdempotencyTTL:  24 * time.Hour,  //Key expires after 24 hours
	}
}
