### 런처 배포 이미지 — gojira/launcher-dist
#
# 역할: **이미 빌드·서명된** 런처 산출물을 공유 볼륨에 배치하고 종료하는 init job.
#       platform-api 가 그 볼륨을 /api/v2/launcher/* 로 서빙한다.
#
# ⚠️ 여기서 컴파일하지 않는다 (설계 §10.7 / D14).
#    서버에서 다시 빌드하면 **서명한 바이너리와 실제 배포 바이너리가 달라져 서명이 무의미**해진다.
#    빌드·서명은 사내 GitHub Actions 의 launcher-release 워크플로가 수행하고,
#    산출물(바이너리 + .sha256 + version.txt + release-manifest.json + .sig)을
#    deploy/launcher-dist/ 에 게시한다. 이 이미지는 그것을 담아 나르기만 한다.
#
# build context: 레포 루트.
#
# 산출 레이아웃(Platform-API/handlers/launcher.go 가 읽는 형태):
#   /launcher-dist/<os-subdir>/gojira-launcher[.exe](+.sha256)
#   /launcher-dist/version.txt
#   /launcher-dist/release-manifest.json(+.sig)   ← 서명 체계(§10.6). 없으면 런처가 구형식으로 폴백

FROM alpine:3.20
# deploy/launcher-dist 가 비어 있으면(최초 체크아웃 등) 빈 디렉터리가 들어간다 —
# 그 경우 platform-api 는 version 빈 문자열을 돌려주고 런처는 "업데이트 없음" 으로 넘어간다.
COPY deploy/launcher-dist/ /launcher-dist/
# 매 기동마다 공유 볼륨(/shared)을 산출물로 덮어쓴다 → up -d 시 항상 최신.
CMD ["sh","-c","cp -r /launcher-dist/. /shared/ 2>/dev/null; echo '[launcher-dist] 공유 볼륨 채움:'; ls -R /shared"]
