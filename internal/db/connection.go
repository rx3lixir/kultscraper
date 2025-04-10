package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type ConnectionConfig struct {
	URI            string
	Database       string
	CollectionName string
	Username       string
	Password       string
	Timeout        time.Duration
}

// NewDefaultConfig создает конфигурацию по умолчанию
func NewDefaultConfig(uri, database, collection string) *ConnectionConfig {
	return &ConnectionConfig{
		URI:            uri,
		Database:       database,
		CollectionName: collection,
		Timeout:        10 * time.Second,
	}
}

// ConnectMongo устанавливает соединение с MongoDB
func ConnectMongo(ctx context.Context, config *ConnectionConfig) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	clientOptions := options.Client().ApplyURI(config.URI)

	// Устанавливаем учетные данные, если есть
	if config.Username != "" && config.Password != "" {
		clientOptions.SetAuth(options.Credential{
			Username: config.Username,
			Password: config.Password,
		})
	}

	// Подключаемся к MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}

	// Проверяем подключение
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return nil, err
	}

	return client, nil
}
