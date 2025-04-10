package scraper

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/go-rod/rod"
	"github.com/go-rod/stealth"
	"github.com/rx3lixir/kultscraper/internal/config"
	"github.com/rx3lixir/kultscraper/internal/models"
)

var (
	ErrContextCancelled = errors.New("scraping cancelled due to context timeout")
)

// Scraper интерфейс для скрапинга
type Scraper interface {
	Scrape(ctx context.Context, task config.ScraperTask) (*models.ScrapingResult, error)
	Close() error
}

// RodScraper имплементация Scraper с использованием Rod
type RodScraper struct {
	Browser      *rod.Browser
	Logger       log.Logger
	pagePool     *sync.Pool
	maxPageCount int
	activePages  int
	mu           sync.Mutex
}

// TaskToScrape структура для задачи скрапинга
type TaskToScrape struct {
	Task    config.ScraperTask
	Context context.Context
	Scraper Scraper
	Logger  log.Logger
}

// Execute выполняет задачу скрапинга
func (t TaskToScrape) Execute() (any, error) {
	ctx, cancel := context.WithTimeout(t.Context, 30*time.Second)
	defer cancel()

	res, err := t.Scraper.Scrape(ctx, t.Task)
	if err != nil {
		return nil, err
	}

	t.Logger.Info("Scraped Result", "url", t.Task.URL, "type", t.Task.Type)
	return res, nil
}

// OnError обрабатывает ошибки
func (t TaskToScrape) OnError(err error) {
	t.Logger.Error("Failed to scrape task", "url", t.Task.URL, "error", err)
}

// NewRodScraper создает новый скрапер на основе Rod
func NewRodScraper(browser *rod.Browser, logger log.Logger, maxPages int) *RodScraper {
	if maxPages <= 0 {
		maxPages = 10 // Значение по умолчанию
	}

	scraper := &RodScraper{
		Browser:      browser,
		Logger:       logger,
		maxPageCount: maxPages,
		pagePool: &sync.Pool{
			New: func() any {
				page, err := stealth.Page(browser)
				if err != nil {
					logger.Error("Failed to create page", "error", err)
					return nil
				}
				return page
			},
		},
	}

	return scraper
}

// NewTaskToScrape создает новую задачу скрапинга
func NewTaskToScrape(task config.ScraperTask, ctx context.Context, scraper Scraper, logger log.Logger) *TaskToScrape {
	return &TaskToScrape{
		Task:    task,
		Context: ctx,
		Scraper: scraper,
		Logger:  logger,
	}
}

// getPage получает страницу из пула или создает новую
func (r *RodScraper) getPage() (*rod.Page, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.activePages >= r.maxPageCount {
		return nil, errors.New("maximum number of active pages reached")
	}

	page := r.pagePool.Get()
	if page == nil {
		return nil, errors.New("failed to get page from pool")
	}

	r.activePages++
	return page.(*rod.Page), nil
}

// releasePage возвращает страницу в пул
func (r *RodScraper) releasePage(page *rod.Page) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Очищаем страницу перед возвратом в пул
	page.MustNavigate("about:blank")

	r.pagePool.Put(page)
	r.activePages--
}

// Scrape выполняет скрапинг страницы
func (r *RodScraper) Scrape(ctx context.Context, task config.ScraperTask) (*models.ScrapingResult, error) {
	r.Logger.Info("Scraping", "url", task.URL)

	// Проверяем, отменен ли контекст
	select {
	case <-ctx.Done():
		return nil, ErrContextCancelled
	default:
	}

	// Получаем страницу из пула
	page, err := r.getPage()
	if err != nil {
		r.Logger.Error("Failed to get page", "error", err)
		return nil, err
	}
	defer r.releasePage(page)

	// Навигация с учетом контекста
	navCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	err = page.Context(navCtx).Navigate(task.URL)
	if err != nil {
		r.Logger.Error("Failed to navigate to page", "url", task.URL, "error", err)
		return nil, err
	}

	// Ожидание загрузки страницы с таймаутом
	err = page.Context(ctx).WaitLoad()
	if err != nil {
		r.Logger.Error("Failed to wait for page load", "url", task.URL, "error", err)
		return nil, err
	}

	data := make(map[string]string)

	for key, selector := range task.Selectors {
		// Проверяем, отменен ли контекст
		select {
		case <-ctx.Done():
			r.Logger.Warn("Scraping canceled during selector processing", "key", key)
			return models.NewScrapingResult(task.URL, task.Type, task.Name, data), ctx.Err()
		default:
		}

		if selector == "" {
			data[key] = ""
			continue
		}

		// Устанавливаем таймаут для поиска элементов
		elemCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		elements, err := page.Context(elemCtx).Elements(selector)
		cancel()

		if err != nil || len(elements) == 0 {
			r.Logger.Warn("No elements found", "selector", selector, "page", task.URL)
			data[key] = ""
			continue
		}

		var texts []string
		for _, element := range elements {
			// Проверяем, отменен ли контекст
			select {
			case <-ctx.Done():
				r.Logger.Warn("Scraping canceled during element processing", "key", key)
				return models.NewScrapingResult(task.URL, task.Type, task.Name, data), ctx.Err()
			default:
			}

			// Устанавливаем таймаут для получения текста
			textCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			text, err := element.Context(textCtx).Text()
			cancel()

			if err != nil {
				r.Logger.Warn("Failed to get text from element", "selector", selector, "error", err)
				continue
			}

			texts = append(texts, text)
		}

		data[key] = strings.Join(texts, "\n")
		r.Logger.Info("Successfully scraped", "key", key, "count", len(texts))
	}

	result := models.NewScrapingResult(task.URL, task.Type, task.Name, data)

	return result, nil
}

// Close закрывает ресурсы скрапера
func (r *RodScraper) Close() error {
	// Закрываем браузер при завершении
	return r.Browser.Close()
}
