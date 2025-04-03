package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/rx3lixir/kultscraper/internal/config"
	"github.com/rx3lixir/kultscraper/internal/lib/logger"
	"github.com/rx3lixir/kultscraper/internal/lib/work"
	"github.com/rx3lixir/kultscraper/internal/scraper"
)

const (
	numWorkers       = 6
	maxPages         = 10
	defaultTimeout   = 3 * time.Minute
	scrapeTimeout    = 30 * time.Second
	gracefulShutdown = 10 * time.Second
)

func main() {
	logger := logger.InitLogger()
	logger.Info("Starting KultScraper")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Error loading config file", "error", err)
		os.Exit(1)
	}

	// Создаем контекст, который будет отменен по сигналу
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Гарантированный вызов функции отмены

	// Обработка сигналов завершения
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signalCh
		logger.Info("Received signal", "signal", sig)
		cancel()
	}()

	// Загружаем задачи
	tasks, err := config.LoadTasks(cfg.ConfigPath)
	if err != nil {
		logger.Error("Failed to load tasks", "error", err)
		os.Exit(1)
	}
	logger.Info("Loaded tasks", "count", len(tasks))

	// Инициализируем браузер
	browser := rod.New().MustConnect()
	defer browser.Close()

	// Создаем скрапер
	s := scraper.NewRodScraper(browser, *logger, maxPages)
	defer s.Close()

	// Создаем пул работников
	pool, err := work.NewPool(numWorkers, len(tasks))
	if err != nil {
		logger.Error("Failed to create worker pool", "error", err)
		os.Exit(1)
	}

	// Запускаем пул
	if err := pool.Start(ctx); err != nil {
		logger.Error("Failed to start worker pool", "error", err)
		os.Exit(1)
	}

	// Гарантируем остановку пула
	defer func() {
		// Создаем контекст с таймаутом для грейсфул шатдауна
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdown)
		defer shutdownCancel() // Гарантированный вызов функции отмены

		// Запускаем горутину для остановки пула
		done := make(chan struct{})
		go func() {
			pool.Stop()
			close(done)
		}()

		// Ожидаем либо завершения остановки, либо таймаута
		select {
		case <-done:
			logger.Info("Pool stopped gracefully")
		case <-shutdownCtx.Done():
			logger.Warn("Pool shutdown timed out")
		}
	}()

	// Добавляем задачи в пул
	for _, task := range tasks {
		// Создаем таймаут контекст для каждой задачи
		taskCtx, taskCancel := context.WithTimeout(ctx, scrapeTimeout)
		scraperTask := scraper.NewTaskToScrape(task, taskCtx, s, *logger)

		if err := pool.AddTask(scraperTask); err != nil {
			logger.Error("Failed to add task", "url", task.URL, "error", err)
			taskCancel() // Отменяем контекст, если не удалось добавить задачу
			continue
		}

		// Отмена контекста задачи будет выполнена после завершения задачи
		// или при отмене родительского контекста через defer
		go func(cancel context.CancelFunc) {
			// Ожидание завершения задачи или отмены родительского контекста
			select {
			case <-taskCtx.Done():
				// Задача завершена или превысила таймаут
			case <-ctx.Done():
				// Родительский контекст отменен
			}
			cancel() // Отменяем контекст задачи
		}(taskCancel)
	}

	// Обрабатываем результаты
	resultsProcessed := 0
	for {
		select {
		case res, ok := <-pool.Results():
			if !ok {
				logger.Info("Results channel closed")
				return
			}

			logger.Info("Got result", "data", res)
			resultsProcessed++

			// Если все задачи обработаны, выходим
			if resultsProcessed >= len(tasks) {
				logger.Info("All tasks completed", "count", resultsProcessed)
				return
			}

		case <-ctx.Done():
			logger.Info("Context cancelled, stopping")
			return
		}
	}
}
