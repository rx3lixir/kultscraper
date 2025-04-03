package config

import (
	"encoding/json"
	"os"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	Timeout    string
	ConfigPath string
	OutputPath string
}

func LoadConfig() (*AppConfig, error) {
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	return &AppConfig{
		Timeout:    os.Getenv("SCRAPER_TIMEOUT"),
		ConfigPath: os.Getenv("CONFIG_PATH"),
		OutputPath: os.Getenv("OUTPUT_PATH"),
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
