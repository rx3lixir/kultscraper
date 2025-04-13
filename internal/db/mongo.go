package db

import (
	"context"
	"errors"
	"time"

	"github.com/rx3lixir/kultscraper/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DefaultTimeout = 10 * time.Second
)

var (
	ErrNotFound      = errors.New("document not found")
	ErrInvalidID     = errors.New("invalid document ID")
	ErrNilCollection = errors.New("collection is nil")
)

// ScraperRepository определяет интерйес для работы с данными скраппинга
type ScraperRepository interface {
	GetAllResults(ctx context.Context) ([]*models.ScrapingResult, error)
	GetResultByID(ctx context.Context, id string) (*models.ScrapingResult, error)
	GetResultByType(ctx context.Context, scraperType string) (*models.ScrapingResult, error)

	SaveResult(ctx context.Context, result *models.ScrapingResult) (string, error)
	SaveResults(ctx context.Context, result []*models.ScrapingResult) ([]string, error)

	UpdateResult(ctx context.Context, result *models.ScrapingResult) error
	DeleteResult(ctx context.Context, id string) error

	Close() error
}

// MongoScraperRepo имплементирует интерфейс ScraperRepository
type MongoScraperRepo struct {
	client     *mongo.Client
	collection *mongo.Collection
}

// NewMongoScraperRepo создает новый репозиторий скраппинга
func NewMongoScraperRepo(client *mongo.Client, dbname, collectionName string) (*MongoScraperRepo, error) {
	if client == nil {
		return nil, errors.New("Mongo client is nil")
	}

	collection := client.Database(dbname).Collection(collectionName)
	if collection == nil {
		return nil, ErrNilCollection
	}

	repo := &MongoScraperRepo{
		client:     client,
		collection: collection,
	}

	// Создаем индексы для более быстрого поиска
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Создаем индекс по полю type
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "type", Value: 1},
		},
	})
	if err != nil {
		return nil, err
	}

	// Создаем составной индекс по type и url
	_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "type", Value: 1},
			{Key: "url", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}

	return repo, nil
}

// GetAllResults возвращает все результаты скраппинга
func (r *MongoScraperRepo) GetAllResults(ctx context.Context) ([]*models.ScrapingResult, error) {
	if r.collection == nil {
		return nil, ErrNilCollection
	}

	timeout, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	cursor, err := r.collection.Find(timeout, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []*models.ScrapingResult

	if err := cursor.All(timeout, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetResultByID возвращает результат скраппинга по ID
func (r *MongoScraperRepo) GetResultByID(ctx context.Context, id string) (*models.ScrapingResult, error) {
	if r.collection == nil {
		return nil, ErrNilCollection
	}

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrInvalidID
	}

	timeout, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	var result models.ScrapingResult

	err = r.collection.FindOne(timeout, bson.M{"_id": objID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &result, nil
}

func (r *MongoScraperRepo) GetResultsByType(ctx context.Context, scraperType string) ([]*models.ScrapingResult, error) {
	if r.collection == nil {
		return nil, ErrNilCollection
	}

	timeout, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	cursor, err := r.collection.Find(timeout, bson.M{"type": scraperType})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var results []*models.ScrapingResult
	if err := cursor.All(timeout, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// SaveResult сохраняет один результат скраппинга
func (r *MongoScraperRepo) SaveResult(ctx context.Context, result *models.ScrapingResult) (string, error) {
	if r.collection == nil {
		return "", ErrNilCollection
	}

	timeout, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// Проверяем существует ли уже документ с таким URL и типом
	filter := bson.M{"url": result.URL, "type": result.Type}

	var existing models.ScrapingResult

	err := r.collection.FindOne(timeout, filter).Decode(&existing)

	if err == nil {
		// Документ существует, обновляем его
		result.ID = existing.ID
		result.CreatedAt = existing.CreatedAt
		result.UpdatedAt = time.Now()

		update := bson.M{
			"$set": bson.M{
				"name":       result.Name,
				"data":       result.Data,
				"updated_at": result.UpdatedAt,
				"metadata":   result.Metadata,
			},
		}

		_, err = r.collection.UpdateOne(timeout, bson.M{"_id": existing.ID}, update)
		if err != nil {
			return "", err
		}

		return existing.ID.Hex(), nil
	} else if err == mongo.ErrNoDocuments {
		// Документ не существует, создаем новый
		result.ID = primitive.NewObjectID()
		result.CreatedAt = time.Now()
		result.UpdatedAt = result.CreatedAt

		_, err = r.collection.InsertOne(timeout, result)
		if err != nil {
			return "", err
		}

		return result.ID.Hex(), nil
	}

	// Какая-то другая непредвиденная ошибка
	return "", err
}

// SaveResults сохраняет несколько результатов скраппинга
func (r *MongoScraperRepo) SaveResults(ctx context.Context, results []*models.ScrapingResult) ([]string, error) {
	if r.collection == nil {
		return nil, ErrNilCollection
	}

	if len(results) == 0 {
		return []string{}, nil
	}

	var ids []string
	for _, result := range results {
		id, err := r.SaveResult(ctx, result)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// UpdateResult обновляет результат скраппинга
func (r *MongoScraperRepo) UpdateResult(ctx context.Context, result *models.ScrapingResult) error {
	if r.collection == nil {
		return ErrNilCollection
	}

	if result.ID.IsZero() {
		return ErrInvalidID
	}

	timeout, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"name":       result.Name,
			"data":       result.Data,
			"updated_at": result.UpdatedAt,
			"metadata":   result.Metadata,
		},
	}

	_, err := r.collection.UpdateOne(timeout, bson.M{"_id": result.ID}, update)
	return err
}

// DeleteResult удаляет результат скраппинга по ID
func (r *MongoScraperRepo) DeleteResult(ctx context.Context, id string) error {
	if r.collection == nil {
		return ErrNilCollection
	}

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return ErrInvalidID
	}

	timeout, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	_, err = r.collection.DeleteOne(timeout, bson.M{"_id": objID})
	return err
}

// Close закрывает соединение с базой данных
func (r *MongoScraperRepo) Close() error {
	if r.client != nil {
		return r.client.Disconnect(context.Background())
	}
	return nil
}
