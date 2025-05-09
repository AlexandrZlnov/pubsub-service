package config

import (
	"time"
)

type Config struct {
	GRPCPort         string        `mapstructure:"GRPC_PORT"`
	ShutdownTimeout  time.Duration `mapstructure:"SHUTDOWN_TIMEOUT"`
	MaxSubscriptions int           `mapstructure:"MAX_SUBSCRIPTIONS"`
	MaxEventQueue    int           `mapstructure:"MAX_EVENT_QUEUE"`
}

func NewDefaultConfig() *Config {
	return &Config{
		GRPCPort:         "50051",
		ShutdownTimeout:  10 * time.Second,
		MaxSubscriptions: 1000,
		MaxEventQueue:    100,
	}
}
