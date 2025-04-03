package scraper

import (
	"context"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/go-rod/rod"
	"gitub.com/rx3lixir/kultscraper/internal/config"

	"github.com/go-rod/stealth"
)

type Scraper interface {
	Scrape(ctx context.Context, task TaskToScrape) (map[string]string, error)
}

type RodScraper struct {
	Browser *rod.Browser
	Logger  log.Logger
}

type TaskToScrape struct {
	Task    config.ScraperTask
	Context context.Context
	Scraper RodScraper
	Logger  *log.Logger
}

func (t *TaskToScrape) Execute() (interface{}, error) {
	res, err := t.Scraper.Scrape(t.Context, t.Task)
	if err != nil {
		return nil, err
	}

	t.Logger.Infof("Scraped Result for %v: %s", t.Task.URL, res)
	return res, nil
}

func (s *TaskToScrape) OnError(err error) {
	s.Logger.Error("Failed to scrape a task", "error")
}

func NewRodScraper(browser *rod.Browser, logger log.Logger) *RodScraper {
	return &RodScraper{
		Browser: browser,
		Logger:  logger,
	}
}

func NewTaskToScrape(task config.ScraperTask, ctx context.Context, scraper RodScraper, logger log.Logger) *TaskToScrape {
	return &TaskToScrape{
		Task:    task,
		Context: ctx,
		Scraper: scraper,
		Logger:  &logger,
	}
}

func (r *RodScraper) Scrape(ctx context.Context, task config.ScraperTask) (map[string]string, error) {
	r.Logger.Info("Scraping", "url:", task.URL)

	select {
	case <-ctx.Done():
		r.Logger.Error("Context time out", "error")
		return nil, ctx.Err()
	default:
	}

	page, err := stealth.Page(r.Browser)
	if err != nil {
		r.Logger.Error("Failed to create page", "error")
		return nil, err
	}

	select {
	case <-ctx.Done():
		r.Logger.Error("Scraping canceled during navigation to page: %w", ctx.Err())
		return nil, ctx.Err()
	default:
	}

	if err = page.Navigate(task.URL); err != nil {
		r.Logger.Error("Failed to navigate to page: %w", err)
		return nil, err
	}

	page.MustWaitLoad()

	results := make(map[string]string)
	results["URL"] = task.URL
	results["Type"] = task.Type

	for key, selector := range task.Selectors {
		select {
		case <-ctx.Done():
			r.Logger.Warn("Scraping canceled during selector processing", "key", key)
			return results, ctx.Err()
		default:
		}

		if selector == "" {
			results[key] = ""
			continue
		}

		elements, err := page.Elements(selector)
		if err != nil || len(elements) == 0 {
			r.Logger.Warn("No elements found", "selector", "error:", err)
			results[key] = ""
			continue
		}

		var texts []string
		for _, element := range elements {
			select {
			case <-ctx.Done():
				r.Logger.Warn("Scraping canceled due to context timeout processing element", "key", key)
				return nil, err
			default:
			}

			textFromElement, err := element.Text()
			if err != nil {
				r.Logger.Warn("Failed to get text from element", "selector:", selector)
				continue
			}

			texts = append(texts, textFromElement)
		}

		results[key] = strings.Join(texts, "\n")
		r.Logger.Info("Successfully scraped", "key:", key, "count:", len(texts))
	}

	return results, nil
}
