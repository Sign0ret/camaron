"""
ONNX YOLOv8 inference pipeline.
Runs YOLOv8 object detection using ONNX Runtime.
Person-only filtering (COCO class 0).
"""

import os
from datetime import datetime

import numpy as np
from PIL import Image

from models import DetectionResults, Detection, BBox, Point
from nms import nms

try:
    import onnxruntime as ort
except ImportError:  # pragma: no cover
    ort = None


def letterbox(
    frame: np.ndarray,
    new_shape: int = 640,
    color: tuple = (114, 114, 114),
) -> tuple:
    """
    Resize and pad image to target size while preserving aspect ratio.
    Returns (padded_image, scale, pad_left, pad_top).
    """
    h, w = frame.shape[:2]
    scale = min(new_shape / h, new_shape / w)
    new_h, new_w = int(round(h * scale)), int(round(w * scale))

    # Resize
    if (new_w, new_h) != (w, h):
        img = Image.fromarray(frame)
        img = img.resize((new_w, new_h), Image.BILINEAR)
        img = np.array(img)
    else:
        img = frame.copy()

    # Calculate padding
    pad_top = (new_shape - new_h) // 2
    pad_bottom = new_shape - new_h - pad_top
    pad_left = (new_shape - new_w) // 2
    pad_right = new_shape - new_w - pad_left

    # Pad
    img = np.pad(
        img,
        ((pad_top, pad_bottom), (pad_left, pad_right), (0, 0)),
        mode="constant",
        constant_values=color,
    )

    return img, scale, pad_left, pad_top


class YOLOInferencePipeline:
    """
    Wraps an ONNX YOLOv8 model.
    Returns DetectionResults with track_id=None; tracking is applied separately.
    """

    def __init__(
        self,
        model_path: str = "models/yolov8n.onnx",
        device: str = "cpu",
        confidence: float = 0.3,
        nms_iou: float = 0.7,
        input_size: int = 640,
    ):
        if ort is None:
            raise RuntimeError(
                "onnxruntime is not installed. Install it: pip install onnxruntime"
            )

        if not os.path.exists(model_path):
            raise RuntimeError(
                f"ONNX model not found at {model_path}. "
                f"Please export or download the model and place it at {model_path}"
            )

        self.confidence = confidence
        self.nms_iou = nms_iou
        self.input_size = input_size
        self.frame_counter = 0

        # Setup ONNX Runtime
        providers = ["CPUExecutionProvider"]
        sess_options = ort.SessionOptions()
        sess_options.graph_optimization_level = (
            ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        )
        sess_options.intra_op_num_threads = int(os.getenv("ONNX_THREADS", "1"))

        print(f"[inference] Loading ONNX model {model_path} on {device}...")
        self.session = ort.InferenceSession(
            model_path, sess_options, providers=providers
        )
        self.input_name = self.session.get_inputs()[0].name
        input_shape = self.session.get_inputs()[0].shape
        print(f"[inference] ONNX model ready. Input shape: {input_shape}")

    def preprocess(self, frame: np.ndarray) -> tuple:
        """Preprocess frame for ONNX inference."""
        img, scale, pad_left, pad_top = letterbox(frame, self.input_size)
        # HWC -> CHW, normalize to 0-1
        img = img.transpose(2, 0, 1)
        img = np.ascontiguousarray(img, dtype=np.float32) / 255.0
        img = np.expand_dims(img, axis=0)  # Add batch dimension
        return img, scale, pad_left, pad_top

    def postprocess(
        self,
        outputs: np.ndarray,
        scale: float,
        pad_left: int,
        pad_top: int,
        orig_h: int,
        orig_w: int,
    ) -> list[Detection]:
        """Parse ONNX outputs and apply NMS + person filtering."""
        output = outputs[0]  # Remove batch dimension

        # YOLOv8 ONNX output can be (1, 84, 8400), (1, 8400, 84), or (84, 8400)
        if output.shape[0] == 84 and len(output.shape) == 2:
            # (84, 8400)
            output = output.T  # -> (8400, 84)
        elif output.shape[1] == 84 and len(output.shape) == 2:
            # (8400, 84)
            pass
        elif output.shape[0] == 1 and output.shape[2] == 84:
            # (1, 8400, 84)
            output = output[0]
        else:
            print(f"[inference] WARNING: Unexpected ONNX output shape: {output.shape}")
            return []

        # Split into boxes and scores
        boxes = output[:, :4]       # xywh, center format, pixel coords on 640x640
        scores = output[:, 4:]      # 80 class probabilities

        # Get class IDs and confidences
        class_ids = np.argmax(scores, axis=1)
        confidences = np.max(scores, axis=1)

        # Filter by confidence
        valid_mask = confidences >= self.confidence
        boxes = boxes[valid_mask]
        confidences = confidences[valid_mask]
        class_ids = class_ids[valid_mask]

        if len(boxes) == 0:
            return []

        # Person-only filter (COCO class 0)
        person_mask = class_ids == 0
        boxes = boxes[person_mask]
        confidences = confidences[person_mask]

        if len(boxes) == 0:
            return []

        # Convert xywh (center) to xyxy
        xyxy_boxes = np.copy(boxes)
        xyxy_boxes[:, 0] = boxes[:, 0] - boxes[:, 2] / 2  # x1
        xyxy_boxes[:, 1] = boxes[:, 1] - boxes[:, 3] / 2  # y1
        xyxy_boxes[:, 2] = boxes[:, 0] + boxes[:, 2] / 2  # x2
        xyxy_boxes[:, 3] = boxes[:, 1] + boxes[:, 3] / 2  # y2

        # Scale back to original image size
        xyxy_boxes[:, [0, 2]] -= pad_left
        xyxy_boxes[:, [1, 3]] -= pad_top
        xyxy_boxes[:, [0, 2]] /= scale
        xyxy_boxes[:, [1, 3]] /= scale

        # Clip to image bounds
        xyxy_boxes[:, [0, 2]] = np.clip(xyxy_boxes[:, [0, 2]], 0, orig_w)
        xyxy_boxes[:, [1, 3]] = np.clip(xyxy_boxes[:, [1, 3]], 0, orig_h)

        # NMS
        keep_indices = nms(xyxy_boxes, confidences, self.nms_iou)

        detections = []
        for i in keep_indices:
            x1, y1, x2, y2 = xyxy_boxes[i]
            cx = (x1 + x2) / 2.0
            cy = (y1 + y2) / 2.0
            detections.append(
                Detection(
                    track_id=None,
                    class_name="person",
                    confidence=float(confidences[i]),
                    bounding_box=BBox(
                        x1=float(x1), y1=float(y1), x2=float(x2), y2=float(y2)
                    ),
                    center_point=Point(x=float(cx), y=float(cy)),
                )
            )

        return detections

    def process_frame(self, frame: np.ndarray) -> DetectionResults:
        self.frame_counter += 1
        timestamp = datetime.utcnow()

        orig_h, orig_w = frame.shape[:2]
        input_tensor, scale, pad_left, pad_top = self.preprocess(frame)

        # Run inference
        outputs = self.session.run(None, {self.input_name: input_tensor})

        # Postprocess
        detections = self.postprocess(
            outputs[0], scale, pad_left, pad_top, orig_h, orig_w
        )

        return DetectionResults(
            frame_id=self.frame_counter,
            timestamp=timestamp,
            detections=detections,
        )
