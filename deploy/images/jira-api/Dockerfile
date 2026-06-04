### JIRA-API mock 빌드
#
# wheel 은 gojira/build-cache-py 에 미리 빌드돼 있어(폐쇄망 자체 빌드용) 외부 PyPI
# 호출 없이 --no-index --find-links 로 오프라인 설치한다. requirements.txt 변경 시
# build-cache-py 를 갱신해야 한다. build context: 레포 루트.

FROM gojira/build-cache-py:latest AS wheels
# /wheels 에 requirements 의 wheel 이 적재돼 있음 (외부 PyPI 불필요)

FROM python:3.12-slim
RUN useradd -u 10001 -m app
WORKDIR /app
COPY --from=wheels /wheels /wheels
COPY JIRA-API/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir --no-index --find-links=/wheels -r /app/requirements.txt && \
    rm -rf /wheels
COPY JIRA-API/ /app/
RUN chown -R app:app /app
USER app
EXPOSE 8000
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
