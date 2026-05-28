FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y \
    libavformat-dev libavcodec-dev libavdevice-dev \
    libavutil-dev libswscale-dev libswresample-dev \
    pkg-config gcc g++ wget \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy ONNX model and source code
COPY models/ ./models/
COPY main.py .
COPY turso.py .
COPY models.py .
COPY inference.py .
COPY bytetrack.py .
COPY nms.py .
COPY tracker.py .
COPY camera_rules.json .

CMD ["python", "main.py"]
