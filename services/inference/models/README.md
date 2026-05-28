# ONNX Models

Place your exported YOLOv8 ONNX model files here.

## Export Instructions

Run the following on your local machine to generate `yolov8n.onnx`:

```bash
pip install ultralytics
python -c "from ultralytics import YOLO; YOLO('yolov8n.pt').export(format='onnx', imgsz=640, opset=12)"
mv yolov8n.onnx services/inference/models/
```

### PyTorch 2.6+ Workaround

If you get a `WeightsUnpicklingError` with PyTorch 2.6+, use:

```bash
python -c "
import torch
from ultralytics.nn.tasks import DetectionModel
torch.serialization.add_safe_globals([DetectionModel])
from ultralytics import YOLO
YOLO('yolov8n.pt').export(format='onnx', imgsz=640, opset=12)
"
```

## Configuration

The inference worker expects the model path to match the `YOLO_MODEL`
environment variable (default: `models/yolov8n.onnx`).
