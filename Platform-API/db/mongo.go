package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"platform-api/config"
)

var (
	client   *mongo.Client
	database *mongo.Database
	// gojiraDB — GoJIRA 소유 DB 핸들. DocRepo 가 projects 컬렉션(과제의 documents 필드)을
	// 직접 갱신하는 역방향 결합 전용 (D8: platform DB 분리 후에도 projects 는 gojira DB 에 남는다).
	// 신규 코드는 이 핸들을 쓰지 말 것 — 제품 데이터는 제품 API 경유가 원칙.
	gojiraDB *mongo.Database
)

func Init() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	client, err = mongo.Connect(options.Client().ApplyURI(config.MongoURI))
	if err != nil {
		log.Fatalf("MongoDB connect error: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping error: %v", err)
	}

	database = client.Database(config.MongoDB)
	gojiraDB = client.Database(config.GojiraMongoDB)
	log.Printf("MongoDB connected: %s (db=%s, gojiraDB=%s)", config.MongoURI, config.MongoDB, config.GojiraMongoDB)
}
