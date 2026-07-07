package db

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// seedIssueNum — issues 컬렉션 max 스캔으로 `{code}-NNN` 의 다음 번호를 계산한다.
// counters 컬렉션 첫 초기화(seed) 전용 — 이후 발급은 counters.go 의 원자적
// NextIssueNum 이 담당한다 (localOnly 생성은 issues 에 upsert 되지 않으므로
// max 스캔만으로는 중복 발급됨. 또한 최신 이슈 삭제 후 생성 시 키 재사용 버그도 있음).
// Python 측 skills/helpers.py:get_next_issue_num 과 동일 규칙.
func seedIssueNum(ctx context.Context, code string) (int, error) {
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

// seedSubkeyNum — issues 컬렉션 max 스캔으로 `{prefix}_NNN` 의 다음 번호(int)를 계산한다.
// counters 컬렉션 첫 초기화(seed) 전용 — 이후 발급은 counters.go 의 원자적
// NextSubkey 가 담당한다 (seedIssueNum 주석 참고).
// Python 측 skills/helpers.py:next_subkey 와 동일 규칙.
func seedSubkeyNum(ctx context.Context, code, prefix string) (int, error) {
	pat := fmt.Sprintf("^%s_(\\d+)$", regexp.QuoteMeta(prefix))
	filter := bson.M{
		"key":    bson.M{"$regex": fmt.Sprintf("^%s-", regexp.QuoteMeta(code))},
		"subkey": bson.M{"$regex": pat},
	}
	cur, err := Issues().Find(ctx, filter)
	if err != nil {
		return 0, err
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
	return mx + 1, nil
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
