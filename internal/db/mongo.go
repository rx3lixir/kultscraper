package db

import (
	"go.mongodb.org/mongo-driver/mongo"
)

// ScraperRepository определяет интерйес для работы с данными скраппинга
type ScraperRepository interface {
	GetAllResults()
	GetResultByID()
	SaveAllResults()
	SaveResultByID()
}

// MongoScraperRepo имплементирует интерфейс ScraperRepository
type MongoScraperRepo struct {
	collection *mongo.Collection
}

// NewMongoScraperRepo создает новый репозиторий рецептов
func NewMongoScraperRepo(client *mongo.Client, dbname, collectionName string) *MongoScraperRepo {
	return &MongoScraperRepo{
		collection: client.Database(dbname).Collection(collectionName),
	}
}
