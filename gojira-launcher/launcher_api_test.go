package main

import (
	"runtime"
	"testing"
)

func TestRouteState(t *testing.T) {
	t.Run("미등록 서비스는 installable", func(t *testing.T) {
		if st, _ := routeState(nil); st != "installable" {
			t.Fatalf("got %q", st)
		}
	})

	t.Run("요건 없으면 available", func(t *testing.T) {
		if st, _ := routeState(&ServiceSpec{Name: "x"}); st != "available" {
			t.Fatalf("got %q", st)
		}
	})

	t.Run("OS 불일치는 unsupported", func(t *testing.T) {
		other := "windows"
		if runtime.GOOS == "windows" {
			other = "linux"
		}
		st, missing := routeState(&ServiceSpec{Name: "x", OS: []string{other}})
		if st != "unsupported" || len(missing) == 0 {
			t.Fatalf("got %q missing=%v", st, missing)
		}
	})

	t.Run("요건 불충족은 unsupported + 사유", func(t *testing.T) {
		st, missing := routeState(&ServiceSpec{Name: "x", Requires: []string{"존재하지-않는-요건"}})
		if st != "unsupported" || len(missing) != 1 {
			t.Fatalf("got %q missing=%v", st, missing)
		}
	})
}

func TestCheckRequirement(t *testing.T) {
	t.Run("모르는 키는 불충족 처리", func(t *testing.T) {
		// 조용히 통과시키면 서비스가 요건 없이 기동해버린다.
		if c := checkRequirement("아무키"); c.OK {
			t.Fatal("모르는 키가 충족으로 나왔다")
		}
	})

	t.Run("git 은 이 환경에 있다", func(t *testing.T) {
		if c := checkRequirement("git"); !c.OK {
			t.Skipf("git 없음 — 환경 문제: %s", c.Why)
		}
	})

	t.Run("office-com 은 비 Windows 에서 불충족", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows 에서는 실제 레지스트리를 본다")
		}
		if c := checkRequirement("office-com"); c.OK {
			t.Fatal("비 Windows 에서 office-com 이 충족으로 나왔다")
		}
	})
}

func TestSupervisorRegisterSnapshot(t *testing.T) {
	sv := newSupervisor(&Server{})
	sv.register(ServiceSpec{Name: "a", Version: "1.0.0", builtin: true})
	sv.register(ServiceSpec{Name: "a", Version: "9.9.9"}) // 중복 등록은 무시
	sv.register(ServiceSpec{Name: "b", Version: "1.0.0", Requires: []string{"없는요건"}})

	snap := sv.snapshot()
	if len(snap) != 2 {
		t.Fatalf("서비스 수 %d (중복 등록이 덮어썼나)", len(snap))
	}
	for _, e := range snap {
		// 내장은 프로세스가 없으므로 running, 외부는 아직 안 띄웠으니 stopped.
		want := string(stateStopped)
		if e["builtin"] == true {
			want = string(stateRunning)
		}
		if e["name"] == "b" {
			want = string(stateStopped) // 요건 불충족이지만 외부 서비스라 기동 전 상태
		}
		if e["state"] != want {
			t.Errorf("%v: state=%v, want %v", e["name"], e["state"], want)
		}
		if e["name"] == "a" && e["version"] != "1.0.0" {
			t.Errorf("중복 등록이 기존 항목을 덮었다: %v", e["version"])
		}
		if e["name"] == "b" {
			req := e["requires"].(map[string]any)
			if req["ok"] != false {
				t.Errorf("요건 불충족이 반영되지 않았다: %v", req)
			}
		}
	}
}

// 내장 서비스는 프로세스가 없으므로 ensureRunning 이 즉시 성공해야 한다.
func TestEnsureRunningBuiltin(t *testing.T) {
	sv := newSupervisor(&Server{})
	sv.register(ServiceSpec{Name: "builtin-x", builtin: true})
	if err := sv.ensureRunning(t.Context(), "builtin-x"); err != nil {
		t.Fatalf("내장 서비스에서 실패: %v", err)
	}
	if err := sv.ensureRunning(t.Context(), "없는서비스"); err == nil {
		t.Fatal("미등록 서비스가 성공했다")
	}
}
