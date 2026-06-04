### Python wheel 캐시 이미지 — 폐쇄망 자체 빌드용 (jira-api)
#
# 외부망에서 한 번만 빌드해 wheel 을 /wheels 에 적재 → docker save 로 build-cache.tar.
# 이후 jira-api Dockerfile 의 builder stage 를 `python:3.12-slim` →
# `gojira/build-cache-py:latest` 로 바꾸면 외부 PyPI 호출 없이
# `pip install --no-index --find-links=/wheels` 로 오프라인 설치 가능.
#
# 갱신 시점: JIRA-API/requirements.txt 변경 시.
#
# build context: 레포 루트.

FROM python:3.12-slim
WORKDIR /build
COPY JIRA-API/requirements.txt .
RUN pip install --no-cache-dir --upgrade pip wheel && \
    pip wheel --no-cache-dir --wheel-dir=/wheels -r requirements.txt

# 캐시 위치 확인용 (디버깅 — 정리 가능)
RUN ls -la /wheels | head -5 || true
