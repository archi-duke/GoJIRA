package main

// 요건 점검 (설계서 §9.3).
//
// 서비스는 service.json 의 `requires` 로 필요한 것을 선언하고, 런처는 기동 전과
// `/launcher/capabilities` 계산 시 이를 점검한다. 불충족이면 그 경로는 `unsupported`
// 로 표시되고, 호출되면 `424 requirement_missing` 으로 응답한다(§7.2).
//
// 키 추가는 런처 업데이트 사항이므로 최소한으로 유지한다 — 새 키가 필요하면
// 서비스가 자체 점검 후 /health 에서 ready=false 로 알리는 쪽을 먼저 검토할 것.

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// requirementCheck — 요건 키 하나의 점검 결과.
type requirementCheck struct {
	Key string `json:"key"`
	OK  bool   `json:"ok"`
	Why string `json:"why,omitempty"` // 불충족 사유(표시용)
}

// reqCache — 점검 결과 캐시. exec 호출이 섞여 있어 매 요청 반복하면 느리다.
// 설치·환경 변화를 반영하려고 TTL 을 둔다.
var reqCache = struct {
	sync.Mutex
	at   time.Time
	vals map[string]requirementCheck
}{vals: map[string]requirementCheck{}}

const reqCacheTTL = 60 * time.Second

// checkRequirement — 요건 키 하나를 점검한다(캐시 적용).
func checkRequirement(key string) requirementCheck {
	reqCache.Lock()
	if time.Since(reqCache.at) < reqCacheTTL {
		if v, ok := reqCache.vals[key]; ok {
			reqCache.Unlock()
			return v
		}
	} else {
		reqCache.vals = map[string]requirementCheck{}
		reqCache.at = time.Now()
	}
	reqCache.Unlock()

	v := evalRequirement(key)

	reqCache.Lock()
	if reqCache.at.IsZero() {
		reqCache.at = time.Now()
	}
	reqCache.vals[key] = v
	reqCache.Unlock()
	return v
}

func evalRequirement(key string) requirementCheck {
	switch key {
	case "git":
		return binCheck(key, "git")
	case "code":
		return binCheck(key, "code")
	case "opencode|claude":
		// 에이전트는 둘 중 하나만 있으면 된다.
		if _, err := exec.LookPath("opencode"); err == nil {
			return requirementCheck{Key: key, OK: true}
		}
		if _, err := exec.LookPath("claude"); err == nil {
			return requirementCheck{Key: key, OK: true}
		}
		return requirementCheck{Key: key, Why: "opencode·claude 모두 PATH 에 없음"}
	case "powershell":
		if runtime.GOOS != "windows" {
			return requirementCheck{Key: key, Why: "Windows 전용 요건"}
		}
		return binCheck(key, "powershell")
	case "office-com":
		return checkOfficeCOM(key)
	default:
		// 모르는 키는 "불충족" 으로 둔다 — 조용히 통과시키면 서비스가 엉뚱하게 기동한다.
		return requirementCheck{Key: key, Why: "런처가 모르는 요건 키 (런처 업데이트 필요)"}
	}
}

func binCheck(key, bin string) requirementCheck {
	if _, err := exec.LookPath(bin); err != nil {
		return requirementCheck{Key: key, Why: bin + " 를 PATH 에서 찾지 못함"}
	}
	return requirementCheck{Key: key, OK: true}
}

// checkOfficeCOM — Word/PowerPoint COM 등록 여부(§12.2 ragaas-converter 용).
// Windows 외에서는 불충족 — converter 가 macOS/Linux 에서 unsupported 로 표시되는 근거다.
func checkOfficeCOM(key string) requirementCheck {
	if runtime.GOOS != "windows" {
		return requirementCheck{Key: key, Why: "Windows 전용 (Office COM)"}
	}
	// HKCR 에 ProgID 가 등록돼 있는지만 본다 — Office 를 실제로 띄우지 않는다(느리고 부작용).
	cmd := exec.Command("reg", "query", `HKCR\PowerPoint.Application`)
	if err := cmd.Run(); err != nil {
		return requirementCheck{Key: key, Why: "PowerPoint.Application COM 미등록"}
	}
	return requirementCheck{Key: key, OK: true}
}

// checkRequirements — 여러 요건을 점검해 (전체 충족 여부, 불충족 목록) 을 돌려준다.
func checkRequirements(keys []string) (bool, []requirementCheck) {
	var missing []requirementCheck
	for _, k := range keys {
		if c := checkRequirement(strings.TrimSpace(k)); !c.OK {
			missing = append(missing, c)
		}
	}
	return len(missing) == 0, missing
}
