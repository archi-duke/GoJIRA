### Agent-Service 빌드 — wheelhouse 캐시로 폐쇄망 친화

FROM python:3.12-slim AS builder
WORKDIR /build
COPY Agent-Service/requirements.txt .
RUN pip install --no-cache-dir --upgrade pip wheel && \
    pip wheel --no-cache-dir --wheel-dir=/wheels -r requirements.txt

FROM python:3.12-slim
RUN useradd -u 10001 -m app
WORKDIR /app
COPY --from=builder /wheels /wheels
COPY Agent-Service/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir --no-index --find-links=/wheels -r /app/requirements.txt && \
    rm -rf /wheels
COPY Agent-Service/ /app/
RUN chown -R app:app /app
USER app
ENV AGENT_SERVICE_PORT=9002
EXPOSE 9002
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "9002"]
