### GoJIRA 게이트웨이 — 단일 TLS 종단 리버스 프록시 (nginx)
#
# 모든 브라우저 대면 트래픽(2 React 앱 + 2 API + pumlex)의 단일 진입점.
# URL_SCHEME=https 면 인증서로 TLS 종단, 아니면 평문. 경로 기반으로 백엔드 라우팅
# (locations.conf). 백엔드 컨테이너는 호스트에 노출되지 않고 내부 네트워크로만 접근.
#
# 실제 listen 포트는 GATEWAY_PORT (entrypoint 가 설정 생성). build context = 레포 루트.
FROM nginx:alpine
COPY deploy/images/gateway/locations.conf /etc/nginx/gojira-locations.conf
COPY deploy/images/gateway/entrypoint.sh /docker-entrypoint.d/40-gateway.sh
RUN chmod +x /docker-entrypoint.d/40-gateway.sh
# 정보용 — 실제 포트는 GATEWAY_PORT 환경변수로 런타임 결정.
EXPOSE 8080
