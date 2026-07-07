package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func Notices() *mongo.Collection { return database.Collection("notices") }

func InitNoticeIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := Notices().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "status", Value: 1}, {Key: "startDate", Value: 1}, {Key: "endDate", Value: 1}},
	})
	if err != nil {
		log.Printf("index notices: %v", err)
	}
	log.Println("notice indexes initialized")
}

// FindAllNotices 앱 스코프의 전체 공지 조회 (isActive만, 최신순) — D8 앱 스코핑
func FindAllNotices(ctx context.Context, app string) ([]bson.M, error) {
	filter := appFilter(app)
	filter["isActive"] = true
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	cursor, err := Notices().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// FindActiveNotices 앱 스코프의 현재 게시중인 공지 조회 (사용자 팝업용)
func FindActiveNotices(ctx context.Context, app string) ([]bson.M, error) {
	now := time.Now()
	filter := appFilter(app)
	filter["isActive"] = true
	filter["status"] = "active"
	filter["startDate"] = bson.M{"$lte": now}
	filter["endDate"] = bson.M{"$gte": now}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})
	cursor, err := Notices().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func FindNoticeByID(ctx context.Context, id string) (bson.M, error) {
	return findByID(ctx, Notices(), id)
}

func InsertNotice(ctx context.Context, doc bson.M) error {
	return insertDoc(ctx, Notices(), doc)
}

func UpdateNotice(ctx context.Context, id string, updates bson.M) error {
	return updateDoc(ctx, Notices(), id, updates)
}

func SoftDeleteNotice(ctx context.Context, id, by string) error {
	return softDelete(ctx, Notices(), id, by)
}

// IncrementNoticeViews 조회수 증가
func IncrementNoticeViews(ctx context.Context, id string) error {
	_, err := Notices().UpdateOne(ctx, bson.M{"_id": toID(id)}, bson.M{"$inc": bson.M{"views": 1}})
	return err
}

// UpdateNoticeStatuses 게시 기간 기반 상태 자동 갱신
func UpdateNoticeStatuses(ctx context.Context) {
	now := time.Now()
	// 게시전 → 게시중
	Notices().UpdateMany(ctx,
		bson.M{"isActive": true, "status": "draft", "startDate": bson.M{"$lte": now}, "endDate": bson.M{"$gte": now}},
		bson.M{"$set": bson.M{"status": "active", "updatedAt": now}},
	)
	// 게시중 → 게시종료
	Notices().UpdateMany(ctx,
		bson.M{"isActive": true, "status": "active", "endDate": bson.M{"$lt": now}},
		bson.M{"$set": bson.M{"status": "expired", "updatedAt": now}},
	)
}
