// gojira-sign — 런처 매니페스트·서비스 카탈로그 ed25519 서명 도구 (설계서 §10.6).
//
// 폐쇄망이라 keyless(Sigstore) 불가 → 키 기반 ed25519. Go 표준 crypto/ed25519 만 사용.
// 사내 GitHub Actions 의 서명 job(launcher-release.yml / catalog-sign.yml)에서 호출한다.
//
// 사용:
//
//	gojira-sign keygen                      개인키(base64) + 공개키(base64) 생성 — 오프라인 PC 1회
//	gojira-sign sign   -in F [-out F.sig]   $SIGNING_KEY 로 파일 서명
//	gojira-sign verify -in F -sig F.sig -pub <base64>
//
// 개인키는 환경변수 SIGNING_KEY(base64) 로만 받는다 — argv 노출 금지(ps 로 보임).
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		cmdKeygen()
	case "sign":
		cmdSign(os.Args[2:])
	case "verify":
		cmdVerify(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `gojira-sign — ed25519 서명 도구

  keygen                                   키쌍 생성 (개인키는 environment secret 으로 등록 후 원본 파기)
  sign   -in <file> [-out <file.sig>]      SIGNING_KEY(base64) 로 서명
  verify -in <file> -sig <file.sig> -pub <base64>

서명 원문은 파일 바이트 그대로다(정규화 없음) — 서명된 파일을 그대로 배포할 것.
`)
}

func cmdKeygen() {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		die("키 생성 실패: %v", err)
	}
	fmt.Printf("PRIVATE (SIGNING_KEY 로 등록 후 원본 파기):\n%s\n\n", base64.StdEncoding.EncodeToString(priv))
	fmt.Printf("PUBLIC (런처 소스에 커밋):\n%s\n", base64.StdEncoding.EncodeToString(pub))
}

func cmdSign(args []string) {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	in := fs.String("in", "", "서명할 파일")
	out := fs.String("out", "", "서명 출력 (기본: <in>.sig)")
	_ = fs.Parse(args)
	if *in == "" {
		die("-in 필요")
	}
	keyB64 := os.Getenv("SIGNING_KEY")
	if keyB64 == "" {
		die("SIGNING_KEY 환경변수 필요 (base64 ed25519 개인키)")
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		die("SIGNING_KEY 형식 오류 — base64 ed25519 개인키(%d bytes) 여야 함", ed25519.PrivateKeySize)
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		die("읽기 실패: %v", err)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(key), data)
	dst := *out
	if dst == "" {
		dst = *in + ".sig"
	}
	if err := os.WriteFile(dst, []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		die("서명 쓰기 실패: %v", err)
	}
	fmt.Printf("서명 완료: %s → %s\n", *in, dst)
}

func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	in := fs.String("in", "", "원문 파일")
	sigPath := fs.String("sig", "", "서명 파일")
	pubB64 := fs.String("pub", "", "공개키 (base64)")
	_ = fs.Parse(args)
	if *in == "" || *sigPath == "" || *pubB64 == "" {
		die("-in, -sig, -pub 모두 필요")
	}
	pub, err := base64.StdEncoding.DecodeString(*pubB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		die("공개키 형식 오류")
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		die("원문 읽기 실패: %v", err)
	}
	sigRaw, err := os.ReadFile(*sigPath)
	if err != nil {
		die("서명 읽기 실패: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(trimSpace(string(sigRaw)))
	if err != nil {
		die("서명 형식 오류")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		die("검증 실패 — 서명 불일치")
	}
	fmt.Println("검증 성공")
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "gojira-sign: "+format+"\n", a...)
	os.Exit(1)
}
