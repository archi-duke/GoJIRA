package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func notifColl() *mongo.Collection {
	return database.Collection("notifications")
}

func InitNotificationIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	notifColl().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "read", Value: 1}},
		Options: options.Index(),
	})
	// TTL 7일
	notifColl().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(7 * 24 * 60 * 60),
	})
}

// SaveNotification 알림 저장 (미접속 사용자용)
func SaveNotification(ctx context.Context, userId, ntype, projectCode, message string, data bson.M) error {
	_, err := notifColl().InsertOne(ctx, bson.M{
		"userId":      userId,
		"type":        ntype,
		"projectCode": projectCode,
		"message":     message,
		"data":        data,
		"read":        false,
		"createdAt":   time.Now(),
	})
	return err
}

// FindUnreadNotifications 미읽은 알림 조회
func FindUnreadNotifications(ctx context.Context, userId string) ([]bson.M, error) {
	cursor, err := notifColl().Find(ctx, bson.M{"userId": userId, "read": false})
	if err != nil {
		return nil, err
	}
	var results []bson.M
	err = cursor.All(ctx, &results)
	return results, err
}

// MarkNotificationsRead 알림 읽음 처리
func MarkNotificationsRead(ctx context.Context, userId string) error {
	_, err := notifColl().UpdateMany(ctx, bson.M{"userId": userId, "read": false}, bson.M{"$set": bson.M{"read": true}})
	return err
}
