package db

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// NextIssueNum — `{code}-NNN` 형식의 다음 번호 반환.
// Mongo 캐시에서 현재 최대 번호 + 1. YAML 진실 원천과 캐시가 일치한다는 전제.
// Python 측 skills/helpers.py:get_next_issue_num 과 동일 규칙.
func NextIssueNum(ctx context.Context, code string) (int, error) {
	filter := bson.M{"key": bson.M{"$regex": fmt.Sprintf("^%s-", regexp.QuoteMeta(code))}}
	cur, err := Issues().Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	prefix := code + "-"
	mx := 0
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		k, _ := doc["key"].(string)
		if len(k) > len(prefix) {
			if n, err := strconv.Atoi(k[len(prefix):]); err == nil && n > mx {
				mx = n
			}
		}
	}
	return mx + 1, nil
}

// NextSubkey — `{prefix}_NNN` (3자리 zero-pad) 다음 값 반환.
// Python 측 skills/helpers.py:next_subkey 와 동일.
func NextSubkey(ctx context.Context, code, prefix string) (string, error) {
	pat := fmt.Sprintf("^%s_(\\d+)$", regexp.QuoteMeta(prefix))
	filter := bson.M{
		"key":    bson.M{"$regex": fmt.Sprintf("^%s-", regexp.QuoteMeta(code))},
		"subkey": bson.M{"$regex": pat},
	}
	cur, err := Issues().Find(ctx, filter)
	if err != nil {
		return "", err
	}
	defer cur.Close(ctx)
	rx := regexp.MustCompile(pat)
	mx := 0
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		sk, _ := doc["subkey"].(string)
		if m := rx.FindStringSubmatch(sk); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > mx {
				mx = n
			}
		}
	}
	return fmt.Sprintf("%s_%03d", prefix, mx+1), nil
}

// SubkeyPrefix — issueType + reqtype → subkey prefix.
// 매핑 없음 → "" (호출 측이 필수 여부 결정).
// Python _SUBKEY_PREFIX / _REQ_SUBKEY_PREFIX 와 동일.
func SubkeyPrefix(issueType, reqtype string) string {
	if issueType == "Requirement" {
		switch reqtype {
		case "FR":
			return "FR"
		case "NFR":
			return "QA"
		case "CNST":
			return "CNST"
		}
		return ""
	}
	switch issueType {
	case "VOS":
		return "VOS"
	case "Feature":
		return "UC"
	case "Architectural Design":
		return "ADSN"
	case "Detailed Design":
		return "DDSN"
	}
	return ""
}
