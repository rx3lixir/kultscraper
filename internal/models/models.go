package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Sraping result - модель для созранения результатов скраппинга в базу данных
type ScrapingResult struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	URL       string             `bson:"url" json:"url"`
	Type      string             `bson:"type" json:"type"`
	Name      string             `bson:"name" json:"name"`
	Data      map[string]string  `bson:"data" json:"data"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
	Metadata  map[string]any     `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

// NewScrapingResult создает новый результат скраппинга
func NewScrapingResult(url, scrapeType, name string, data map[string]string) *ScrapingResult {
	now := time.Now()

	return &ScrapingResult{
		URL:       url,
		Type:      scrapeType,
		Name:      name,
		Data:      data,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]any),
	}
}
