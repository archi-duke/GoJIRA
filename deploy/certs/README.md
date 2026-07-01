# TLS 인증서 디렉토리

`URL_SCHEME=https` 로 호스팅할 때 gateway 컨테이너가 이 디렉토리를 read-only 로
마운트해 TLS 종단에 사용한다. (`.env` 의 `TLS_CERT_DIR` 가 이 경로를 가리킴)

필요한 두 파일:

| 파일 | 내용 |
|---|---|
| `fullchain.pem` | 서버 인증서 + 중간 CA 체인 (leaf → intermediate 순 PEM 연결) |
| `privkey.pem`   | 서버 개인키 (암호화되지 않은 PEM) |

- 인증서의 CN 또는 SAN 은 `.env` 의 `GATEWAY_HOST` 와 일치해야 브라우저 경고가 없다.
- **개인키를 포함하므로 git 커밋 금지** — `.gitignore` 가 `deploy/certs/*` 를 제외한다.
- `URL_SCHEME=http` 면 이 디렉토리는 비어 있어도 무방하다.

## 자체서명 인증서 예시 (테스트용)

```sh
openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
  -keyout privkey.pem -out fullchain.pem \
  -subj "/CN=gojira.corp.local" \
  -addext "subjectAltName=DNS:gojira.corp.local"
```

사내 운영은 자체서명 대신 사내 CA 발급 인증서를 권장한다.
