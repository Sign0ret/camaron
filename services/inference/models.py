"""
Typed dataclasses for the inference pipeline.
"""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional


@dataclass
class Point:
    x: float
    y: float


@dataclass
class BBox:
    x1: float
    y1: float
    x2: float
    y2: float


@dataclass
class Detection:
    track_id: Optional[int]
    class_name: str
    confidence: float
    bounding_box: BBox
    center_point: Point


@dataclass
class DetectionResults:
    frame_id: int
    timestamp: datetime
    detections: list[Detection] = field(default_factory=list)


@dataclass
class Line:
    id: str
    name: str
    start: Point
    end: Point
    camera_id: Optional[str] = None


@dataclass
class Zone:
    id: str
    name: str
    polygon: list[Point]
    camera_id: Optional[str] = None


@dataclass
class Event:
    id: str
    camera_id: str
    event_type: str
    timestamp: datetime
    metadata: dict = field(default_factory=dict)
    track_id: Optional[int] = None
