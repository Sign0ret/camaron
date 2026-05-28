"""
Pure NumPy Non-Maximum Suppression.
No OpenCV required.
"""

import numpy as np


def nms(boxes: np.ndarray, scores: np.ndarray, iou_threshold: float) -> list[int]:
    """
    Greedy Non-Maximum Suppression.

    Args:
        boxes: Array of shape (N, 4) in [x1, y1, x2, y2] format.
        scores: Array of shape (N,) with confidence scores.
        iou_threshold: IoU threshold above which boxes are suppressed.

    Returns:
        List of indices to keep.
    """
    if len(boxes) == 0:
        return []

    x1 = boxes[:, 0]
    y1 = boxes[:, 1]
    x2 = boxes[:, 2]
    y2 = boxes[:, 3]
    areas = (x2 - x1) * (y2 - y1)

    order = np.argsort(scores)[::-1]
    keep = []

    while len(order) > 0:
        i = order[0]
        keep.append(int(i))

        if len(order) == 1:
            break

        xx1 = np.maximum(x1[i], x1[order[1:]])
        yy1 = np.maximum(y1[i], y1[order[1:]])
        xx2 = np.minimum(x2[i], x2[order[1:]])
        yy2 = np.minimum(y2[i], y2[order[1:]])

        w = np.maximum(0.0, xx2 - xx1)
        h = np.maximum(0.0, yy2 - yy1)
        inter = w * h

        iou = inter / (areas[i] + areas[order[1:]] - inter + 1e-6)
        mask = iou <= iou_threshold
        order = order[1:][mask]

    return keep
