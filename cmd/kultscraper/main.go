package main

import (
	"context"
	"os"
	"time"

	"github.com/go-rod/rod"
	"gitub.com/rx3lixir/kultscraper/internal/config"
	"gitub.com/rx3lixir/kultscraper/internal/lib/logger"
	"gitub.com/rx3lixir/kultscraper/internal/lib/work"
	"gitub.com/rx3lixir/kultscraper/internal/scraper"
)

const (
	numWorkers = 6
)

func main() {
	logger := logger.InitLogger()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Error occured while loading config file", "error:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*30))
	defer cancel()

	tasks, err := config.LoadTasks(cfg.ConfigPath)
	if err != nil {
		logger.Error("Failed to load tasks", "error", err)
	}

	browser := rod.New()
	if err := browser.Connect(); err != nil {
		logger.Error("Error occured connecting to rod browser", err)
	}
	defer browser.Close()

	s := scraper.NewRodScraper(browser, *logger)

	pool, err := work.NewPool(numWorkers, len(tasks))
	if err != nil {
		logger.Error("Failed to create worker pool", err)
		os.Exit(1)
	}

	pool.Start(ctx)

	for _, task := range tasks {
		scraperTask := scraper.NewTaskToScrape(task, ctx, *s, *logger)
		pool.AddTask(scraperTask)
	}

	go func() {
		for res := range pool.Results() {
			logger.Printf("Got results: %v \n", res)
		}
	}()

	pool.Stop()

	logger.Info("All tasks completed")
}
