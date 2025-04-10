package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	Timeout    string
	ConfigPath string
	OutputPath string
	MongoDB    MongoDBConfig
}

type MongoDBConfig struct {
	URI            string
	Database       string
	Collection     string
	Username       string
	Password       string
	ConnectTimeout time.Duration
}

func LoadConfig() (*AppConfig, error) {
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	// Таймаут подключения к MongoDB
	connectTimeout := 10 * time.Second
	if timeout := os.Getenv("MONGODB_CONNECT_TIMEOUT"); timeout != "" {
		if parsed, err := time.ParseDuration(timeout); err == nil {
			connectTimeout = parsed
		}
	}

	return &AppConfig{
		Timeout:    os.Getenv("SCRAPER_TIMEOUT"),
		ConfigPath: os.Getenv("CONFIG_PATH"),
		OutputPath: os.Getenv("OUTPUT_PATH"),
		MongoDB: MongoDBConfig{
			URI:            os.Getenv("MONGO_URI"),
			Database:       os.Getenv("MONGODB_DATABASE"),
			Collection:     os.Getenv("MONGODB_COLLECTION"),
			Username:       os.Getenv("MONGODB_USERNAME"),
			Password:       os.Getenv("MONGODB_PASSWORD"),
			ConnectTimeout: connectTimeout,
		},
	}, nil
}

func LoadTasks(filePath string) ([]ScraperTask, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var configs []ScraperTask

	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}

	return configs, nil
}

type ScraperTask struct {
	URL       string            `json:"URL"`
	Type      string            `json:"Type"`
	Name      string            `json:"Name"`
	Selectors map[string]string `json:"Selectors"`
}
