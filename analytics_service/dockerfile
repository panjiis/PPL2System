FROM python:3.12-slim
WORKDIR /app

COPY analytics_service/requirements.txt .

RUN pip install --no-cache-dir -r requirements.txt

COPY analytics_service/ .

EXPOSE 50055

CMD ["python", "main.py"]
