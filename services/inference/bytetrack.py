"""
ByteTrack object tracker.
Pure Python/numpy implementation of the BYTE tracking algorithm.
Uses a Kalman filter for motion prediction and the Hungarian algorithm (via scipy)
for optimal track-to-detection assignment.
"""

import numpy as np
from typing import List, Tuple

from models import DetectionResults, Detection, BBox, Point


# ── Helpers ──────────────────────────────────────────────────────────


def _tlwh_to_xyxy(tlwh: np.ndarray) -> np.ndarray:
    """Convert [top, left, width, height] to [x1, y1, x2, y2]."""
    ret = np.copy(tlwh)
    ret[..., 2:] += ret[..., :2]
    return ret


def _xyxy_to_tlwh(xyxy: np.ndarray) -> np.ndarray:
    """Convert [x1, y1, x2, y2] to [top, left, width, height]."""
    ret = np.copy(xyxy)
    ret[..., 2:] -= ret[..., :2]
    return ret


def _iou_batch(bboxes1: np.ndarray, bboxes2: np.ndarray) -> np.ndarray:
    """Compute IoU matrix between two sets of bboxes in xyxy format."""
    bboxes1 = np.expand_dims(bboxes1, 1)  # (N, 1, 4)
    bboxes2 = np.expand_dims(bboxes2, 0)  # (1, M, 4)

    xx1 = np.maximum(bboxes1[..., 0], bboxes2[..., 0])
    yy1 = np.maximum(bboxes1[..., 1], bboxes2[..., 1])
    xx2 = np.minimum(bboxes1[..., 2], bboxes2[..., 2])
    yy2 = np.minimum(bboxes1[..., 3], bboxes2[..., 3])

    w = np.maximum(0.0, xx2 - xx1)
    h = np.maximum(0.0, yy2 - yy1)
    inter = w * h

    area1 = (bboxes1[..., 2] - bboxes1[..., 0]) * (bboxes1[..., 3] - bboxes1[..., 1])
    area2 = (bboxes2[..., 2] - bboxes2[..., 0]) * (bboxes2[..., 3] - bboxes2[..., 1])

    iou = inter / (area1 + area2 - inter + 1e-6)
    return iou


def _linear_assignment(cost_matrix: np.ndarray) -> np.ndarray:
    """Solve linear assignment using the Hungarian algorithm via scipy."""
    try:
        from scipy.optimize import linear_sum_assignment
    except ImportError:
        raise RuntimeError(
            "scipy is not installed. Install it: pip install scipy"
        )

    # linear_sum_assignment minimizes cost, so it works directly on our cost matrix
    row_ind, col_ind = linear_sum_assignment(cost_matrix)
    return np.column_stack([row_ind, col_ind])


# ── Kalman Filter ────────────────────────────────────────────────────


class KalmanFilter:
    """
    Simple linear Kalman filter for bounding box tracking.
    State: [cx, cy, r, h, vcx, vcy, vr, vh]
    Measurement: [cx, cy, r, h]
    """

    def __init__(self):
        ndim, dt = 4, 1.0

        self._motion_mat = np.eye(2 * ndim, 2 * ndim)
        for i in range(ndim):
            self._motion_mat[i, ndim + i] = dt

        self._update_mat = np.eye(ndim, 2 * ndim)

        self._std_weight_position = 1.0 / 20
        self._std_weight_velocity = 1.0 / 160

    def initiate(self, measurement: np.ndarray) -> Tuple[np.ndarray, np.ndarray]:
        """Create a new track from an initial measurement."""
        mean_pos = measurement
        mean_vel = np.zeros_like(mean_pos)
        mean = np.r_[mean_pos, mean_vel]

        std = [
            2 * self._std_weight_position * measurement[3],
            2 * self._std_weight_position * measurement[3],
            1e-2,
            2 * self._std_weight_position * measurement[3],
            10 * self._std_weight_velocity * measurement[3],
            10 * self._std_weight_velocity * measurement[3],
            1e-5,
            10 * self._std_weight_velocity * measurement[3],
        ]
        covariance = np.diag(np.square(std))
        return mean, covariance

    def predict(self, mean: np.ndarray, covariance: np.ndarray) -> Tuple[np.ndarray, np.ndarray]:
        """Run the Kalman filter prediction step."""
        std_pos = [
            self._std_weight_position * mean[3],
            self._std_weight_position * mean[3],
            1e-2,
            self._std_weight_position * mean[3],
        ]
        std_vel = [
            self._std_weight_velocity * mean[3],
            self._std_weight_velocity * mean[3],
            1e-5,
            self._std_weight_velocity * mean[3],
        ]
        motion_cov = np.diag(np.square(np.r_[std_pos, std_vel]))

        mean = np.dot(self._motion_mat, mean)
        covariance = np.linalg.multi_dot((
            self._motion_mat, covariance, self._motion_mat.T
        )) + motion_cov

        return mean, covariance

    def project(self, mean: np.ndarray, covariance: np.ndarray) -> Tuple[np.ndarray, np.ndarray]:
        """Project state distribution to measurement space."""
        std = [
            self._std_weight_position * mean[3],
            self._std_weight_position * mean[3],
            1e-2,
            self._std_weight_position * mean[3],
        ]
        innovation_cov = np.diag(np.square(std))

        mean = np.dot(self._update_mat, mean)
        covariance = np.linalg.multi_dot((
            self._update_mat, covariance, self._update_mat.T
        ))
        return mean, covariance + innovation_cov

    def update(
        self,
        mean: np.ndarray,
        covariance: np.ndarray,
        measurement: np.ndarray,
    ) -> Tuple[np.ndarray, np.ndarray]:
        """Run the Kalman filter correction step."""
        projected_mean, projected_cov = self.project(mean, covariance)

        # Kalman gain via solve for numerical stability
        b = np.dot(covariance, self._update_mat.T).T  # (4, 8)
        try:
            x = np.linalg.solve(projected_cov.T, b)  # (4, 8)
        except np.linalg.LinAlgError:
            x = np.linalg.lstsq(projected_cov.T, b, rcond=None)[0]
        kalman_gain = x.T  # (8, 4)

        innovation = measurement - projected_mean
        new_mean = mean + np.dot(kalman_gain, innovation)
        new_covariance = covariance - np.linalg.multi_dot((
            kalman_gain, projected_cov, kalman_gain.T
        ))
        return new_mean, new_covariance


# ── Single Track ─────────────────────────────────────────────────────


class STrack:
    """A single target track."""

    def __init__(self, tlwh: np.ndarray, score: float):
        self._tlwh = np.asarray(tlwh, dtype=np.float32)
        self.score = score
        self.kalman_filter = None
        self.mean: np.ndarray | None = None
        self.covariance: np.ndarray | None = None
        self.track_id = 0
        self.is_activated = False
        self.state = "Tentative"
        self.time_since_update = 0
        self.hits = 0

    # ── State machine ────────────────────────────────────────────────

    def predict(self):
        """Propagate track state using Kalman filter."""
        assert self.mean is not None and self.kalman_filter is not None
        self.mean, self.covariance = self.kalman_filter.predict(
            self.mean, self.covariance
        )
        self.time_since_update += 1

    def activate(self, kalman_filter: KalmanFilter, frame_id: int, track_id: int):
        """Initialize a new track."""
        self.kalman_filter = kalman_filter
        self.track_id = track_id
        self.mean, self.covariance = self.kalman_filter.initiate(
            self._tlwh_to_xyah(self._tlwh)
        )
        self.time_since_update = 0
        self.hits = 1
        self.state = "Tentative"
        self.is_activated = True

    def reactivate(self, new_track: "STrack", frame_id: int, new_id: bool = False, new_track_id: int = 0):
        """Reactivate a lost track with a new detection."""
        assert self.kalman_filter is not None
        self.mean, self.covariance = self.kalman_filter.update(
            self.mean, self.covariance, self._tlwh_to_xyah(new_track.tlwh)
        )
        self.time_since_update = 0
        self.hits += 1
        if new_id:
            self.track_id = new_track_id
        self.state = "Confirmed"
        self.score = new_track.score

    def update(self, new_track: "STrack", frame_id: int):
        """Update a matched track with a new detection."""
        assert self.kalman_filter is not None
        self.mean, self.covariance = self.kalman_filter.update(
            self.mean, self.covariance, self._tlwh_to_xyah(new_track.tlwh)
        )
        self.time_since_update = 0
        self.hits += 1
        self.state = "Confirmed"
        self.score = new_track.score

    def mark_lost(self):
        self.state = "Lost"

    def mark_removed(self):
        self.state = "Removed"

    # ── Geometry helpers ─────────────────────────────────────────────

    @property
    def tlwh(self) -> np.ndarray:
        """Current bbox in [top, left, width, height] format."""
        if self.mean is None:
            return self._tlwh.copy()
        ret = self.mean[:4].copy()
        ret[2] *= ret[3]           # w = r * h
        ret[:2] -= ret[2:] / 2.0   # tl = c - wh/2
        return ret

    @property
    def tlbr(self) -> np.ndarray:
        """Current bbox in [x1, y1, x2, y2] format."""
        ret = self.tlwh.copy()
        ret[2:] += ret[:2]
        return ret

    @staticmethod
    def _tlwh_to_xyah(tlwh: np.ndarray) -> np.ndarray:
        """Convert tlwh to [center_x, center_y, aspect_ratio, height]."""
        ret = np.copy(tlwh)
        ret[:2] += ret[2:] / 2.0   # cx, cy
        ret[2] /= ret[3]            # r = w / h
        return ret

    def __repr__(self):
        return f"STrack(id={self.track_id}, tlwh={self.tlwh}, state={self.state})"


# ── BYTE Tracker ─────────────────────────────────────────────────────


class ByteTrack:
    """
    BYTE Tracker: multi-object tracker using Kalman filter + Hungarian matching.
    """

    def __init__(
        self,
        track_thresh: float = 0.5,
        match_thresh: float = 0.8,
        track_buffer: int = 30,
    ):
        self.track_thresh = track_thresh
        self.match_thresh = match_thresh
        self.track_buffer = track_buffer
        self.kalman_filter = KalmanFilter()
        self.tracks: List[STrack] = []
        self.track_id_counter = 0
        self.frame_id = 0

    def _next_id(self) -> int:
        self.track_id_counter += 1
        return self.track_id_counter

    # ── Public API ───────────────────────────────────────────────────

    def update(self, detection_results: DetectionResults) -> DetectionResults:
        """
        Update tracker with new detections and return tracked results.
        All returned detections have a persistent track_id assigned.
        """
        self.frame_id += 1

        # Convert Detection objects to STrack objects
        detections = []
        for det in detection_results.detections:
            bbox = np.array([
                det.bounding_box.x1,
                det.bounding_box.y1,
                det.bounding_box.x2 - det.bounding_box.x1,  # w
                det.bounding_box.y2 - det.bounding_box.y1,  # h
            ], dtype=np.float32)
            detections.append(STrack(bbox, det.confidence))

        # Split detections by confidence
        dets_high = [d for d in detections if d.score >= self.track_thresh]
        dets_low = [d for d in detections if self.track_thresh > d.score > 0.1]

        # Predict all existing tracks
        for track in self.tracks:
            track.predict()

        active_tracks = [t for t in self.tracks if t.state != "Removed"]

        # ── First association: high-confidence detections ──────────────
        matched, unmatched_tracks, unmatched_dets_high = self._associate(
            active_tracks, dets_high
        )

        for idx_track, idx_det in matched:
            track = active_tracks[idx_track]
            det = dets_high[idx_det]
            if track.state == "Lost":
                track.reactivate(det, self.frame_id, new_track_id=self._next_id())
            else:
                track.update(det, self.frame_id)

        for idx in unmatched_tracks:
            track = active_tracks[idx]
            if track.state != "Lost":
                track.mark_lost()

        # ── Second association: low-confidence detections ─────────────
        unmatched_active = [
            active_tracks[i] for i in unmatched_tracks
            if active_tracks[i].state != "Lost"
        ]
        matched2, unmatched_tracks2, _ = self._associate(unmatched_active, dets_low)

        for idx_track, idx_det in matched2:
            track = unmatched_active[idx_track]
            det = dets_low[idx_det]
            if track.state == "Lost":
                track.reactivate(det, self.frame_id, new_track_id=self._next_id())
            else:
                track.update(det, self.frame_id)

        for idx in unmatched_tracks2:
            track = unmatched_active[idx]
            if track.state != "Lost":
                track.mark_lost()

        # ── Initialize new tracks ──────────────────────────────────────
        for idx in unmatched_dets_high:
            det = dets_high[idx]
            det.activate(self.kalman_filter, self.frame_id, self._next_id())
            self.tracks.append(det)

        # ── Cleanup ────────────────────────────────────────────────────
        # Remove tracks lost for too long
        self.tracks = [
            t for t in self.tracks
            if t.state != "Removed" and t.time_since_update <= self.track_buffer
        ]

        # Output only tracks that were updated this frame
        output_tracks = [t for t in self.tracks if t.time_since_update == 0]

        # Build DetectionResults
        tracked_detections = []
        for track in output_tracks:
            x1, y1, x2, y2 = map(float, track.tlbr)
            cx = (x1 + x2) / 2.0
            cy = (y1 + y2) / 2.0
            tracked_detections.append(
                Detection(
                    track_id=track.track_id,
                    class_name="person",
                    confidence=float(track.score),
                    bounding_box=BBox(x1=x1, y1=y1, x2=x2, y2=y2),
                    center_point=Point(x=cx, y=cy),
                )
            )

        return DetectionResults(
            frame_id=detection_results.frame_id,
            timestamp=detection_results.timestamp,
            detections=tracked_detections,
        )

    # ── Internal helpers ─────────────────────────────────────────────

    def _associate(
        self,
        tracks: List[STrack],
        detections: List[STrack],
    ) -> Tuple[np.ndarray, List[int], List[int]]:
        """Associate tracks with detections using IoU + Hungarian algorithm."""
        if len(tracks) == 0 or len(detections) == 0:
            return (
                np.empty((0, 2), dtype=int),
                list(range(len(tracks))),
                list(range(len(detections))),
            )

        track_bboxes = np.array([t.tlbr for t in tracks])
        det_bboxes = _tlwh_to_xyxy(np.array([d._tlwh for d in detections]))

        iou_matrix = _iou_batch(track_bboxes, det_bboxes)
        cost_matrix = 1.0 - iou_matrix

        # Reject low-IoU candidates before running Hungarian
        cost_matrix[cost_matrix > 1.0 - self.match_thresh] = 1e5

        matches = _linear_assignment(cost_matrix)

        unmatched_tracks = [i for i in range(len(tracks)) if i not in matches[:, 0]]
        unmatched_detections = [j for j in range(len(detections)) if j not in matches[:, 1]]

        # Filter out matches that were hard-rejected
        valid_matches = []
        for m in matches:
            i, j = int(m[0]), int(m[1])
            if cost_matrix[i, j] < 1e5:
                valid_matches.append([i, j])

        if not valid_matches:
            return (
                np.empty((0, 2), dtype=int),
                list(range(len(tracks))),
                list(range(len(detections))),
            )

        return np.array(valid_matches), unmatched_tracks, unmatched_detections
