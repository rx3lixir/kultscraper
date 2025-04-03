package work

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Executor interface {
	Execute() (interface{}, error)
	OnError(error)
}

type Pool struct {
	numWorkers int
	tasks      chan Executor
	results    chan interface{}
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	started    bool
	mu         sync.Mutex
	logger     Logger // Интерфейс для логирования
}

// Logger - интерфейс для логирования
type Logger interface {
	Info(msg string, keyvals ...interface{})
	Error(msg string, keyvals ...interface{})
	Debug(msg string, keyvals ...interface{})
}

// NoopLogger - реализация Logger, которая ничего не делает
type NoopLogger struct{}

func (n NoopLogger) Info(msg string, keyvals ...interface{})  {}
func (n NoopLogger) Error(msg string, keyvals ...interface{}) {}
func (n NoopLogger) Debug(msg string, keyvals ...interface{}) {}

// NewPool создает новый пул воркеров с заданными параметрами
func NewPool(numWorkers int, taskChannelSize int) (*Pool, error) {
	return NewPoolWithLogger(numWorkers, taskChannelSize, NoopLogger{})
}

// NewPoolWithLogger создает новый пул воркеров с заданными параметрами и логгером
func NewPoolWithLogger(numWorkers int, taskChannelSize int, logger Logger) (*Pool, error) {
	if numWorkers <= 0 || taskChannelSize <= 0 {
		return nil, errors.New("invalid parameters: number of workers and tasks must be more than zero")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pool{
		numWorkers: numWorkers,
		tasks:      make(chan Executor, taskChannelSize),
		results:    make(chan interface{}, taskChannelSize), // Буферизированный канал для результатов
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
	}, nil
}

// Results возвращает канал результатов
func (p *Pool) Results() <-chan interface{} {
	return p.results
}

// Start запускает пул работников
func (p *Pool) Start(parentCtx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return errors.New("pool already started")
	}

	// Создаем новый контекст, который отменяется либо при отмене родительского контекста,
	// либо при явном вызове p.cancel()
	ctx, cancel := context.WithCancel(parentCtx)
	p.ctx, p.cancel = ctx, cancel

	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	p.started = true
	return nil
}

// Stop останавливает пул работников и ожидает завершения всех задач
func (p *Pool) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	p.started = false
	p.mu.Unlock()

	// Отменяем контекст
	p.cancel()

	// Закрываем канал задач, чтобы работники завершились
	close(p.tasks)

	// Ожидаем завершения всех работников
	p.wg.Wait()

	// Закрываем канал результатов
	close(p.results)
}

// AddTask добавляет задачу в пул
func (p *Pool) AddTask(t Executor) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errors.New("pool not started")
	}
	p.mu.Unlock()

	select {
	case p.tasks <- t:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

// worker запускает работника для обработки задач
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	p.logger.Info("Worker started", "worker_id", id)
	startTime := time.Now()
	tasksProcessed := 0

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Info("Worker stopping due to context cancellation",
				"worker_id", id,
				"tasks_processed", tasksProcessed,
				"uptime", time.Since(startTime))
			return
		case task, ok := <-p.tasks:
			if !ok {
				p.logger.Info("Worker stopping due to closed tasks channel",
					"worker_id", id,
					"tasks_processed", tasksProcessed,
					"uptime", time.Since(startTime))
				return
			}

			taskStartTime := time.Now()
			p.logger.Debug("Worker processing task", "worker_id", id)

			res, err := task.Execute()

			if err != nil {
				task.OnError(err)
				p.logger.Error("Worker encountered error processing task",
					"worker_id", id,
					"error", err,
					"task_duration", time.Since(taskStartTime))
				continue
			}

			// Отправляем результат, учитывая возможность отмены контекста
			select {
			case p.results <- res:
				// Успешно отправили результат
				tasksProcessed++
				p.logger.Debug("Worker completed task successfully",
					"worker_id", id,
					"task_duration", time.Since(taskStartTime))
			case <-p.ctx.Done():
				// Контекст был отменен
				p.logger.Info("Worker stopping while sending results due to context cancellation",
					"worker_id", id)
				return
			}
		}
	}
}
