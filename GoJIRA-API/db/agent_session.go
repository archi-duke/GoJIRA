package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func AgentSessions() *mongo.Collection {
	return database.Collection("agent_sessions")
}

func CreateAgentSession(ctx context.Context, sessionID, userID string, sessionContext bson.M) error {
	_, err := AgentSessions().InsertOne(ctx, bson.M{
		"_id":       sessionID,
		"userId":    userID,
		"messages":  []interface{}{},
		"context":   sessionContext,
		"createdAt": time.Now(),
		"updatedAt": time.Now(),
	})
	return err
}

func GetAgentSession(ctx context.Context, sessionID string) (bson.M, error) {
	var doc bson.M
	err := AgentSessions().FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func AppendAgentMessage(ctx context.Context, sessionID string, message bson.M) error {
	_, err := AgentSessions().UpdateOne(ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$push": bson.M{"messages": message},
			"$set":  bson.M{"updatedAt": time.Now()},
		},
	)
	return err
}

func UpdateAgentSessionContext(ctx context.Context, sessionID string, sessionContext bson.M) error {
	_, err := AgentSessions().UpdateOne(ctx,
		bson.M{"_id": sessionID},
		bson.M{"$set": bson.M{"context": sessionContext, "updatedAt": time.Now()}},
	)
	return err
}

func DeleteAgentSession(ctx context.Context, sessionID string) error {
	_, err := AgentSessions().DeleteOne(ctx, bson.M{"_id": sessionID})
	return err
}

// EnsureAgentSessionIndex TTL 인덱스 생성 (1시간 후 자동 삭제)
func EnsureAgentSessionIndex(ctx context.Context) {
	expireAfter := int32(3600)
	AgentSessions().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "updatedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(expireAfter),
	})
}
