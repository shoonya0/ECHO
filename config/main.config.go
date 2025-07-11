package config

import (
	"fmt"
	"gin/objects"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

var (
	_, b, _, _ = runtime.Caller(0)
	basePath   = filepath.Dir(b)
)

func config(configPath string) error {
	if configPath != "" {
		viper.AddConfigPath(configPath)
	} else {
		viper.AddConfigPath(basePath + "/envConfig")
	}
	viper.SetConfigName("dev")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	// for now env for dev ,prod and test are same
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Config file not found, using defaults")
			return err
		}
		return fmt.Errorf("error reading config file: %w", err)
	}

	if err := viper.Unmarshal(&objects.MainConfiguration); err != nil {
		return fmt.Errorf("unable to decode into struct, %w", err)
	}
	return nil
}

func New(configPath string) {
	if err := config(configPath); err != nil {
		fmt.Println("Error loading configuration:", err)
		os.Exit(1)
	}
}
