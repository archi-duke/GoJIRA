package db

// counters — key/subkey 원자적 발급 (Phase 2 후속 수정).
//
// 배경: 기존 NextIssueNum/NextSubkey 는 issues 컬렉션 max 스캔 방식이라
//   1) localOnly 생성(Mongo 무접촉)은 캐시에 upsert 되지 않아 연속 생성 시 max 가
//      올라가지 않고 같은 key/subkey 가 중복 발급되며 (batch-create 전멸),
//   2) 최신 이슈 삭제 후 생성 시 키가 재사용되는 잠재 버그도 있었다.
//
// 해결: gojira DB 의 counters 컬렉션에 시퀀스를 유지하고 FindOneAndUpdate($inc) 로
// 원자적으로 발급한다. 카운터 문서 shape: {_id: "<종류>:<식별자>", seq: <int>}.
//   - issue key : _id = "issuenum:{code}"
//   - subkey    : _id = "subkey:{code}:{prefix}"
// 첫 발급 시(문서 없음)에는 기존 max 스캔 로직(issue_key.go 의 seed 함수)으로
// 기존 프로젝트의 max 에서 이어받아 초기화한다. 동시 초기화 경합은 _id 유니크
// 제약(duplicate key)으로 감지해 $inc 경로로 재시도한다.

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func Counters() *mongo.Collection { return database.Collection("counters") }

// nextSeqWithSeed — counters 문서 {_id:id, seq} 를 원자적으로 +1 하고 결과 seq 를
// 반환한다. 문서가 없으면 seed(ctx) 값(＝"이번에 쓸 번호")으로 InsertOne 초기화한다.
// InsertOne 이 duplicate key 로 실패하면(동시 초기화 경합) $inc 경로로 재시도한다
// (상한 3회 — 정상적으로는 1회 재시도로 충분).
func nextSeqWithSeed(ctx context.Context, id string, seed func(context.Context) (int, error)) (int, error) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 1) 기존 카운터가 있으면 원자적 $inc 후 증가된 seq 반환.
		opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
		var doc struct {
			Seq int `bson:"seq"`
		}
		err := Counters().FindOneAndUpdate(ctx,
			bson.M{"_id": id},
			bson.M{"$inc": bson.M{"seq": 1}},
			opts,
		).Decode(&doc)
		if err == nil {
			return doc.Seq, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return 0, err
		}

		// 2) 문서 없음 — seed 값으로 초기화 시도.
		seedVal, err := seed(ctx)
		if err != nil {
			return 0, err
		}
		_, err = Counters().InsertOne(ctx, bson.M{"_id": id, "seq": seedVal})
		if err == nil {
			return seedVal, nil
		}
		if mongo.IsDuplicateKeyError(err) {
			// 동시 초기화 경합 — 다른 요청이 먼저 초기화했으므로 $inc 경로로 재시도.
			continue
		}
		return 0, err
	}
	return 0, fmt.Errorf("counter %s: init contention exceeded retry limit", id)
}

// NextIssueNum — `{code}-NNN` 형식의 다음 번호를 원자적으로 발급한다("이번에 쓸 번호").
// 첫 발급 시 issues 컬렉션 max 스캔(seedIssueNum)으로 이어받는다.
func NextIssueNum(ctx context.Context, code string) (int, error) {
	return nextSeqWithSeed(ctx, "issuenum:"+code, func(ctx context.Context) (int, error) {
		return seedIssueNum(ctx, code)
	})
}

// NextSubkey — `{prefix}_NNN` (3자리 zero-pad) 다음 값을 원자적으로 발급한다.
// 첫 발급 시 issues 컬렉션 max 스캔(seedSubkeyNum)으로 이어받는다.
func NextSubkey(ctx context.Context, code, prefix string) (string, error) {
	n, err := nextSeqWithSeed(ctx, "subkey:"+code+":"+prefix, func(ctx context.Context) (int, error) {
		return seedSubkeyNum(ctx, code, prefix)
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%03d", prefix, n), nil
}
