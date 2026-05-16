### JIRA-API mock 빌드

FROM python:3.12-slim AS builder
WORKDIR /build
COPY JIRA-API/requirements.txt .
RUN pip install --no-cache-dir --upgrade pip wheel && \
    pip wheel --no-cache-dir --wheel-dir=/wheels -r requirements.txt

FROM python:3.12-slim
RUN useradd -u 10001 -m app
WORKDIR /app
COPY --from=builder /wheels /wheels
COPY JIRA-API/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir --no-index --find-links=/wheels -r /app/requirements.txt && \
    rm -rf /wheels
COPY JIRA-API/ /app/
RUN chown -R app:app /app
USER app
EXPOSE 8000
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
