package db

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// 조직 트리:  root ─ div ─ team
//
//	└ div2 (parent 가 사라진 고아)
func sampleOrgs() []bson.M {
	return []bson.M{
		{"_id": "root", "parentOrgUnitId": ""},
		{"_id": "div", "parentOrgUnitId": "root"},
		{"_id": "team", "parentOrgUnitId": "div"},
		{"_id": "orphan", "parentOrgUnitId": "gone"},
	}
}

func TestExpandOrgAncestors(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"하위 하나 → 상위 체인 전체", []string{"team"}, []string{"team", "div", "root"}},
		{"중간 조직", []string{"div"}, []string{"div", "root"}},
		{"최상위는 그대로", []string{"root"}, []string{"root"}},
		{"입력 순서 유지 + 중복 제거", []string{"team", "div"}, []string{"team", "div", "root"}},
		{"끊긴 체인은 거기서 중단", []string{"orphan"}, []string{"orphan"}},
		{"목록에 없는 입력도 보존", []string{"unknown"}, []string{"unknown"}},
		{"빈 입력", []string{}, []string{}},
	}
	for _, c := range cases {
		got := ExpandOrgAncestors(c.in, sampleOrgs())
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ExpandOrgAncestors(%v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

// 순환 참조가 있는 데이터에서도 무한 루프에 빠지지 않아야 한다.
func TestExpandOrgAncestorsStopsOnCycle(t *testing.T) {
	orgs := []bson.M{
		{"_id": "a", "parentOrgUnitId": "b"},
		{"_id": "b", "parentOrgUnitId": "a"},
	}
	got := ExpandOrgAncestors([]string{"a"}, orgs)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandOrgAncestors = %v, want %v", got, want)
	}
}

// orgUnits._id 는 문자열과 ObjectID 가 섞여 있다. users.orgUnitIds 는 문자열이므로
// ObjectID 조직도 Hex 문자열로 매칭돼야 상위 조직이 붙는다.
func TestExpandOrgAncestorsMixedIDTypes(t *testing.T) {
	oid := bson.NewObjectID()
	orgs := []bson.M{
		{"_id": "root", "parentOrgUnitId": ""},
		{"_id": oid, "parentOrgUnitId": "root"},
		{"_id": "leaf", "parentOrgUnitId": oid},
	}
	got := ExpandOrgAncestors([]string{"leaf"}, orgs)
	want := []string{"leaf", oid.Hex(), "root"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandOrgAncestors = %v, want %v", got, want)
	}
}

func TestToStringSlice(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []string
	}{
		{"[]string", []string{"a", "b"}, []string{"a", "b"}},
		{"bson.A", bson.A{"a", "b"}, []string{"a", "b"}},
		{"[]interface{}", []interface{}{"a", "b"}, []string{"a", "b"}},
		{"문자열 아닌 원소는 버림", bson.A{"a", 1, "b"}, []string{"a", "b"}},
		{"지원하지 않는 타입", 42, nil},
	}
	for _, c := range cases {
		if got := toStringSlice(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: toStringSlice(%v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}
