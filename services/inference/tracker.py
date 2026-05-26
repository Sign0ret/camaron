"""
Event / business-logic tracker.
Consumes structured detections and emits events:
  - line crossing
  - zone intrusion & dwell time
  - after-hours activity
"""

import json
from datetime import datetime, time as dt_time
from collections import defaultdict
from typing import List

from models import DetectionResults, Detection, Event, Line, Zone, Point


class EventTracker:
    """
    Stateless rule loader + stateful per-camera tracker.
    Safe to instantiate once per StreamWorker.
    """

    def __init__(self, rules_path: str = "camera_rules.json"):
        self.rules = self._load_rules(rules_path)
        # Per-camera state
        self._track_positions: dict[str, dict[int, list[tuple[datetime, Point]]]] = defaultdict(
            lambda: defaultdict(list)
        )
        self._last_crossed: dict[str, datetime] = {}   # key: "{cam}_{track}_{line_id}"
        self._in_zone: dict[str, bool] = {}            # key: "{cam}_{track}_{zone_id}"
        self._dwell_start: dict[str, datetime] = {}    # key: "{cam}_{track}_{zone_id}"

    def _load_rules(self, path: str) -> dict:
        try:
            with open(path, "r") as f:
                data = json.load(f)
                print(f"[tracker] Loaded rules from {path}: {list(data.keys())} cameras")
                return data
        except FileNotFoundError:
            print(f"[tracker] WARNING: Rules file not found at {path}. No zones/lines configured.")
            return {}

    def process_detections(
        self, camera_id: str, results: DetectionResults, frame_shape: tuple[int, int, int]
    ) -> list[Event]:
        """
        Process detections for a single frame and emit events.
        frame_shape: (height, width, channels) from the numpy array
        """
        events: list[Event] = []
        timestamp = results.timestamp
        camera_rules = self.rules.get(camera_id, {})

        lines = [
            Line(
                id=l["id"],
                name=l["name"],
                start=Point(**l["start"]),
                end=Point(**l["end"]),
            )
            for l in camera_rules.get("lines", [])
        ]
        zones = [
            Zone(
                id=z["id"],
                name=z["name"],
                polygon=[Point(**p) for p in z["polygon"]],
            )
            for z in camera_rules.get("zones", [])
        ]
        after_hours_cfg = camera_rules.get("after_hours")
        monitored_classes = camera_rules.get("monitored_classes", ["person"])

        is_after_hours = self._is_after_hours(after_hours_cfg)

        for det in results.detections:
            # Update position history for line-crossing calculations
            self._update_position(camera_id, det, timestamp)

            # Line crossing
            for line in lines:
                event = self._check_line_crossing(camera_id, det, line, timestamp)
                if event:
                    events.append(event)

            # Zone intrusion / dwell
            for zone in zones:
                events.extend(self._check_zone(camera_id, det, zone, timestamp))

            # After-hours activity
            if is_after_hours and det.class_name in monitored_classes:
                events.append(
                    Event(
                        id=f"{camera_id}_{det.track_id or 0}_afterhours_{timestamp.isoformat()}",
                        camera_id=camera_id,
                        event_type="after_hours",
                        timestamp=timestamp,
                        metadata={"class": det.class_name, "confidence": round(det.confidence, 3)},
                        track_id=det.track_id,
                    )
                )

        return events

    # ------------------------------------------------------------------ #
    # Internal helpers
    # ------------------------------------------------------------------ #

    def _update_position(self, camera_id: str, det: Detection, timestamp: datetime):
        if det.track_id is None:
            return
        positions = self._track_positions[camera_id][det.track_id]
        positions.append((timestamp, det.center_point))
        # Keep a sliding window of recent positions
        if len(positions) > 10:
            positions.pop(0)

    def _key(self, camera_id: str, track_id: int, obj_id: str) -> str:
        return f"{camera_id}_{track_id}_{obj_id}"

    def _is_after_hours(self, cfg) -> bool:
        if not cfg:
            return False
        now = datetime.now().time()
        start = dt_time.fromisoformat(cfg["start"])
        end = dt_time.fromisoformat(cfg["end"])
        if start < end:
            return start <= now <= end
        else:
            # crosses midnight
            return now >= start or now <= end

    def _check_line_crossing(
        self, camera_id: str, det: Detection, line: Line, timestamp: datetime
    ) -> Event | None:
        if det.track_id is None:
            return None

        positions = self._track_positions[camera_id][det.track_id]
        if len(positions) < 2:
            return None

        prev_point = positions[-2][1]
        curr_point = positions[-1][1]

        if not self._segments_intersect(prev_point, curr_point, line.start, line.end):
            return None

        key = self._key(camera_id, det.track_id, line.id)
        last = self._last_crossed.get(key)
        if last is not None and (timestamp - last).total_seconds() < 2.0:
            return None  # debounce

        self._last_crossed[key] = timestamp
        return Event(
            id=f"{camera_id}_{det.track_id}_linecross_{line.id}_{timestamp.isoformat()}",
            camera_id=camera_id,
            event_type="line_crossing",
            timestamp=timestamp,
            metadata={
                "line_id": line.id,
                "line_name": line.name,
                "class": det.class_name,
                "confidence": round(det.confidence, 3),
            },
            track_id=det.track_id,
        )

    def _check_zone(
        self, camera_id: str, det: Detection, zone: Zone, timestamp: datetime
    ) -> list[Event]:
        events: list[Event] = []
        if det.track_id is None:
            return events

        inside = self._point_in_polygon(det.center_point, zone.polygon)
        key = self._key(camera_id, det.track_id, zone.id)
        was_inside = self._in_zone.get(key, False)

        if inside and not was_inside:
            # Entered zone
            self._in_zone[key] = True
            self._dwell_start[key] = timestamp
            events.append(
                Event(
                    id=f"{camera_id}_{det.track_id}_zoneenter_{zone.id}_{timestamp.isoformat()}",
                    camera_id=camera_id,
                    event_type="zone_intrusion",
                    timestamp=timestamp,
                    metadata={
                        "zone_id": zone.id,
                        "zone_name": zone.name,
                        "class": det.class_name,
                    },
                    track_id=det.track_id,
                )
            )

        elif not inside and was_inside:
            # Exited zone
            self._in_zone[key] = False
            start = self._dwell_start.pop(key, None)
            dwell = (timestamp - start).total_seconds() if start else 0.0
            events.append(
                Event(
                    id=f"{camera_id}_{det.track_id}_zoneexit_{zone.id}_{timestamp.isoformat()}",
                    camera_id=camera_id,
                    event_type="zone_exit",
                    timestamp=timestamp,
                    metadata={
                        "zone_id": zone.id,
                        "zone_name": zone.name,
                        "dwell_seconds": round(dwell, 2),
                        "class": det.class_name,
                    },
                    track_id=det.track_id,
                )
            )

        # Periodic dwell alert every 30 seconds
        if inside and was_inside:
            start = self._dwell_start.get(key)
            if start and (timestamp - start).total_seconds() >= 30.0:
                self._dwell_start[key] = timestamp
                events.append(
                    Event(
                        id=f"{camera_id}_{det.track_id}_zonedwell_{zone.id}_{timestamp.isoformat()}",
                        camera_id=camera_id,
                        event_type="dwell_time",
                        timestamp=timestamp,
                        metadata={
                            "zone_id": zone.id,
                            "zone_name": zone.name,
                            "dwell_seconds": round((timestamp - start).total_seconds(), 2),
                            "class": det.class_name,
                        },
                        track_id=det.track_id,
                    )
                )

        return events

    @staticmethod
    def _segments_intersect(a1: Point, a2: Point, b1: Point, b2: Point) -> bool:
        # Counter-clockwise test
        def ccw(A: Point, B: Point, C: Point) -> bool:
            return (C.y - A.y) * (B.x - A.x) > (B.y - A.y) * (C.x - A.x)

        return ccw(a1, b1, b2) != ccw(a2, b1, b2) and ccw(a1, a2, b1) != ccw(a1, a2, b2)

    @staticmethod
    def _point_in_polygon(point: Point, polygon: list[Point]) -> bool:
        # Ray-casting algorithm
        x, y = point.x, point.y
        n = len(polygon)
        inside = False
        p1x, p1y = polygon[0].x, polygon[0].y
        for i in range(1, n + 1):
            p2x, p2y = polygon[i % n].x, polygon[i % n].y
            if y > min(p1y, p2y):
                if y <= max(p1y, p2y):
                    if x <= max(p1x, p2x):
                        if p1y != p2y:
                            xinters = (y - p1y) * (p2x - p1x) / (p2y - p1y) + p1x
                        if p1x == p2x or x <= xinters:
                            inside = not inside
            p1x, p1y = p2x, p2y
        return inside
