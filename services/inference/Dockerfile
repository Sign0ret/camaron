FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y \
    libavformat-dev libavcodec-dev libavdevice-dev \
    libavutil-dev libswscale-dev libswresample-dev \
    pkg-config gcc wget \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Pre-download YOLOv8n weights so containers don't fetch at runtime.
RUN python -c "from ultralytics import YOLO; YOLO('yolov8n.pt')"

COPY main.py .
COPY turso.py .
COPY models.py .
COPY inference.py .
COPY tracker.py .
COPY camera_rules.json .

CMD ["python", "main.py"]
