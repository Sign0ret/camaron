"""
YOLOv8 + ByteTrack inference pipeline.
Loads the model once per instance; designed so a future worker pool
can share model weights across cameras while keeping tracker state separate.
"""

from datetime import datetime

import numpy as np

from models import DetectionResults, Detection, BBox, Point

# Lazy import so the module can be imported even if ultralytics isn't present
# (e.g. during linting or when building the container)
try:
    from ultralytics import YOLO
except ImportError:  # pragma: no cover
    YOLO = None


class YOLOInferencePipeline:
    """
    Wraps an Ultralytics YOLO model with per-camera tracking state.
    Each StreamWorker should own one instance to keep ByteTrack state isolated.
    """

    def __init__(
        self,
        model_path: str = "yolov8n.pt",
        device: str = "cpu",
        tracker: str = "bytetrack.yaml",
        confidence: float = 0.3,
    ):
        if YOLO is None:
            raise RuntimeError(
                "ultralytics is not installed. Install it: pip install ultralytics"
            )

        self.device = device
        self.tracker = tracker
        self.confidence = confidence
        self.frame_counter = 0

        print(f"[inference] Loading YOLO model {model_path} on {device}...")
        self.model = YOLO(model_path)
        print(f"[inference] YOLO model ready.")

    def process_frame(self, frame: np.ndarray) -> DetectionResults:
        """
        Run tracking inference on a single RGB frame.

        Args:
            frame: np.ndarray with shape (H, W, 3) and dtype uint8, RGB format.

        Returns:
            DetectionResults containing tracked detections.
        """
        self.frame_counter += 1
        timestamp = datetime.utcnow()

        # ultralytics handles RGB/BGR conversion internally
        results = self.model.track(
            source=frame,
            persist=True,
            tracker=self.tracker,
            conf=self.confidence,
            verbose=False,
            device=self.device,
        )

        if not results or results[0].boxes is None or len(results[0].boxes) == 0:
            return DetectionResults(
                frame_id=self.frame_counter,
                timestamp=timestamp,
                detections=[],
            )

        r = results[0]
        boxes = r.boxes
        names = r.names
        detections = []

        for i in range(len(boxes)):
            xyxy = boxes.xyxy[i].cpu().numpy()
            cls_id = int(boxes.cls[i].cpu().numpy())
            conf = float(boxes.conf[i].cpu().numpy())

            track_id = None
            if boxes.id is not None:
                track_id = int(boxes.id[i].cpu().numpy())

            class_name = names.get(cls_id, "unknown")
            x1, y1, x2, y2 = map(float, xyxy)
            cx = (x1 + x2) / 2.0
            cy = (y1 + y2) / 2.0

            detections.append(
                Detection(
                    track_id=track_id,
                    class_name=class_name,
                    confidence=conf,
                    bounding_box=BBox(x1=x1, y1=y1, x2=x2, y2=y2),
                    center_point=Point(x=cx, y=cy),
                )
            )

        return DetectionResults(
            frame_id=self.frame_counter,
            timestamp=timestamp,
            detections=detections,
        )
