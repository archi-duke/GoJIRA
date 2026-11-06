package main

// 서비스 카탈로그 (설계서 §10.3, §10.6).
//
// 런처는 Platform-API 한 곳만 바라본다(D6). 카탈로그는 "어떤 서비스가 있고 어느 버전이며
// 패키지 해시가 무엇인지" 를 담은 목록이고, **카탈로그 키로 서명**돼 있다.
// 서명·만료·다운그레이드 검사를 통과하지 못하면 신규 설치·업데이트에 쓰지 않는다
// (기존 설치는 그대로 둔다 — 서버 장애로 이미 깔린 기능이 죽으면 안 된다).

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// catalogPublicKeys — 카탈로그 검증용 공개키 2슬롯(current, next). §10.6 키 교체 참고.
// 루트 키(rootPublicKeys, signing.go)와 분리된 키다 — 빈도·피해 범위가 다르다(D13).
var catalogPublicKeys = []string{
	"", // current — TODO: 카탈로그 키 생성 후 채울 것 (catalog-signing environment secret 과 쌍)
	"", // next    — 키 교체 시 사용
}

// CatalogPackage — 플랫폼별 패키지.
type CatalogPackage struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// CatalogService — 카탈로그의 서비스 한 항목.
type CatalogService struct {
	Name     string                    `json:"name"`
	Version  string                    `json:"version"`
	Policy   string                    `json:"policy"` // required | optional
	Packages map[string]CatalogPackage `json:"packages"`
	Routes   []Route                   `json:"routes,omitempty"` // 미설치 상태에서 409 판정용(§10.3)
	Requires []string                  `json:"requires,omitempty"`
	OS       []string                  `json:"os,omitempty"`
}

type Catalog struct {
	IssuedAt  string           `json:"issuedAt"`
	ExpiresAt string           `json:"expiresAt"`
	Commit    string           `json:"commit,omitempty"`
	RunID     string           `json:"runId,omitempty"`
	Services  []CatalogService `json:"services"`
}

// catalogCache — 마지막으로 수용한 카탈로그. 조회 실패 시 이전 것을 계속 쓴다.
var catalogCache = struct {
	sync.Mutex
	cat      *Catalog
	fetched  time.Time
	issuedAt string // 다운그레이드 방지 — 이보다 오래된 카탈로그는 거부
}{}

// fetchCatalog — Platform-API 에서 카탈로그와 서명을 받아 검증 후 보관한다.
func (s *Server) fetchCatalog() (*Catalog, error) {
	base := s.cfg.Backend.PlatformAPI
	if base == "" {
		return nil, fmt.Errorf("backend.platformApi 미설정")
	}
	base = strings.TrimRight(base, "/") + "/launcher"

	raw, err := fetchBytes(base + "/catalog")
	if err != nil {
		return nil, fmt.Errorf("카탈로그 조회 실패: %w", err)
	}
	sigRaw, err := fetchBytes(base + "/catalog.sig")
	if err != nil {
		sigRaw = nil // 서명 없음 — 아래 모드 판정에서 처리
	}

	var cat Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return nil, fmt.Errorf("카탈로그 파싱 실패: %w", err)
	}

	if err := acceptCatalog(raw, string(sigRaw), &cat); err != nil {
		return nil, err
	}

	catalogCache.Lock()
	catalogCache.cat, catalogCache.fetched = &cat, time.Now()
	if cat.IssuedAt != "" {
		catalogCache.issuedAt = cat.IssuedAt
	}
	catalogCache.Unlock()
	return &cat, nil
}

// acceptCatalog — 서명·만료·다운그레이드 검사(§10.6 "키·카탈로그 규칙").
func acceptCatalog(raw []byte, sigB64 string, cat *Catalog) error {
	ok, reason := verifyWithKeys(catalogPublicKeys, raw, sigB64)
	if !ok {
		if signatureMode == "require" {
			return fmt.Errorf("카탈로그 서명 검증 실패(%s) — 필수 모드이므로 사용하지 않습니다", reason)
		}
		log.Printf("⚠️  카탈로그 서명 검증 실패(%s) — 경고 모드라 계속 진행합니다", reason)
	}

	// 만료 — 만료된 카탈로그는 신규 설치·업데이트에 쓰지 않는다(기존 설치는 유지).
	if cat.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, cat.ExpiresAt); err == nil && time.Now().After(exp) {
			return fmt.Errorf("카탈로그가 만료되었습니다 (expiresAt=%s)", cat.ExpiresAt)
		}
	}

	// 다운그레이드·재전송 방지 — 마지막으로 수용한 것보다 오래된 카탈로그 거부.
	catalogCache.Lock()
	last := catalogCache.issuedAt
	catalogCache.Unlock()
	if last != "" && cat.IssuedAt != "" {
		prev, err1 := time.Parse(time.RFC3339, last)
		cur, err2 := time.Parse(time.RFC3339, cat.IssuedAt)
		if err1 == nil && err2 == nil && cur.Before(prev) {
			return fmt.Errorf("카탈로그 issuedAt 역행(%s < %s) — 거부", cat.IssuedAt, last)
		}
	}
	return nil
}

// verifyWithKeys — 공개키 슬롯들로 서명을 검증한다(루트·카탈로그 공용).
func verifyWithKeys(keys []string, raw []byte, sigB64 string) (bool, string) {
	sigB64 = strings.TrimSpace(sigB64)
	if sigB64 == "" {
		return false, "서명 없음"
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false, "서명 형식 오류"
	}
	configured := 0
	for i, pk := range keys {
		pk = strings.TrimSpace(pk)
		if pk == "" {
			continue
		}
		configured++
		key, err := base64.StdEncoding.DecodeString(pk)
		if err != nil || len(key) != ed25519.PublicKeySize {
			log.Printf("서명 검증: 공개키 슬롯 %d 형식 오류 — 건너뜀", i)
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(key), raw, sig) {
			return true, ""
		}
	}
	if configured == 0 {
		return false, "공개키 미설정"
	}
	return false, "서명 불일치"
}

// cachedCatalog — 마지막으로 수용한 카탈로그(없으면 nil).
func cachedCatalog() *Catalog {
	catalogCache.Lock()
	defer catalogCache.Unlock()
	return catalogCache.cat
}

// findService — 카탈로그에서 이름으로 찾는다.
func (c *Catalog) findService(name string) *CatalogService {
	if c == nil {
		return nil
	}
	for i := range c.Services {
		if c.Services[i].Name == name {
			return &c.Services[i]
		}
	}
	return nil
}

// syncCatalog — 카탈로그를 받아 required 서비스를 설치하고 always 서비스를 기동한다(§10.5).
// 기동 시 1회 + update.intervalMinutes 주기로 돈다. 실패는 전부 비치명적이다.
func (s *Server) syncCatalog() {
	run := func() {
		cat, err := s.fetchCatalog()
		if err != nil {
			// 비치명적이지만 조용히 넘기면 안 된다 — 카탈로그가 없으면 웹에는
			// "설치할 서비스 없음" 으로만 보이고, 서버 게시 누락인지 네트워크
			// 문제인지 구분할 수 없다.
			log.Printf("⚠️  카탈로그 동기화 실패 — 로컬 서비스를 설치할 수 없습니다: %v", err)
			s.catMu.Lock()
			s.catErr = err.Error()
			s.catMu.Unlock()
			return
		}
		s.catMu.Lock()
		s.catErr = ""
		s.catMu.Unlock()
		for i := range cat.Services {
			cs := &cat.Services[i]
			if cs.Policy != "required" {
				continue // optional 은 웹이 요청할 때 설치(§10.5)
			}
			if _, ok := cs.Packages[platformKey()]; !ok {
				continue // 이 플랫폼용 패키지가 없는 서비스는 조용히 건너뛴다
			}
			spec, err := s.installService(cs, nil)
			if err != nil {
				log.Printf("required 서비스 %s 설치 실패: %v", cs.Name, err)
				continue
			}
			s.sup.register(*spec)
		}
		// always 서비스 기동 — on-demand 는 첫 요청 때 뜬다.
		for _, spec := range installedServices() {
			if spec.Run.Mode == "always" {
				if err := s.sup.start(spec.Name); err != nil {
					log.Printf("서비스 %s 기동 실패: %v", spec.Name, err)
				}
			}
		}
	}

	run()
	iv := s.cfg.Update.IntervalMinutes
	if iv < 0 {
		return
	}
	if iv == 0 {
		iv = 60
	}
	t := time.NewTicker(time.Duration(iv) * time.Minute)
	defer t.Stop()
	for range t.C {
		run()
	}
}
