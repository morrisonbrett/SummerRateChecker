package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Discord Discord `mapstructure:"discord"`
	Morpho  Morpho  `mapstructure:"morpho"`
	Monitor Monitor `mapstructure:"monitor"`
}

type Discord struct {
	Token   string `mapstructure:"token"`
	GuildID string `mapstructure:"guild_id"`
}

type Morpho struct {
	APIURL string `mapstructure:"api_url"`
}

type Monitor struct {
	CheckIntervalMinutes int `mapstructure:"check_interval_minutes"`
}

func Load() (*Config, error) {
	// Load .env file if it exists
	godotenv.Load()

	// Set up viper
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Set up environment variable mapping
	viper.SetEnvPrefix("SUMMER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("morpho.api_url", "https://blue-api.morpho.org/graphql")
	viper.SetDefault("monitor.check_interval_minutes", 60)

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// Config file not found is OK - we can use env vars
		fmt.Println("No config file found, using environment variables")
	} else {
		fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Debug: print token validation
	token := strings.TrimSpace(config.Discord.Token)
	config.Discord.Token = token // Clean up any whitespace

	fmt.Printf("Token length: %d\n", len(token))
	fmt.Printf("Token starts with: %s\n", func() string {
		if len(token) > 3 {
			return token[:3]
		}
		return token
	}())

	// Validate token format
	if len(token) < 50 {
		fmt.Println("WARNING: Token seems too short")
	}
	if !strings.Contains(token, ".") {
		fmt.Println("WARNING: Token doesn't contain expected dots")
	}

	return &config, nil
}
