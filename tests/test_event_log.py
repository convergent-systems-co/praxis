"""RED-phase tests for the append-only event log (T2).

EventLog persists Events as JSONL, assigning `seq` itself (never trusting a
caller-supplied value) and rejecting a duplicate `event_id` outright, so a
caller retry after a crash can never double-apply an event. `read_all` on a
freshly re-opened EventLog over the same directory must reconstruct the
exact ordered list previously appended -- this is what lets a restarted
process resume purely from persisted events.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_runtime.events import Event, EventLog, EventLogError


def _make_event(*, event_id: str, node_id: str = "node-a", seq: int = 0) -> Event:
    return Event(
        spec_version="1.0.0",
        seq=seq,
        run_id="run-1",
        node_id=node_id,
        event_type="transition-attempted",
        payload={"detail": "example"},
        event_id=event_id,
    )


def test_sequential_appends_get_increasing_seq(tmp_path: Path) -> None:
    log = EventLog(tmp_path)

    first = log.append(_make_event(event_id="evt-1"))
    second = log.append(_make_event(event_id="evt-2"))
    third = log.append(_make_event(event_id="evt-3"))

    assert [first.seq, second.seq, third.seq] == [0, 1, 2]


def test_duplicate_event_id_raises_and_does_not_write_second_line(tmp_path: Path) -> None:
    log = EventLog(tmp_path)
    log.append(_make_event(event_id="evt-1"))

    with pytest.raises(EventLogError):
        log.append(_make_event(event_id="evt-1", node_id="node-b"))

    assert len(log.read_all()) == 1

    log_path = tmp_path / "events.jsonl"
    on_disk_lines = [line for line in log_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    assert len(on_disk_lines) == 1

    reopened = EventLog(tmp_path)
    assert len(reopened.read_all()) == 1


def test_read_all_after_reopening_reconstructs_same_ordered_list(tmp_path: Path) -> None:
    log = EventLog(tmp_path)
    log.append(_make_event(event_id="evt-1"))
    log.append(_make_event(event_id="evt-2"))

    reopened = EventLog(tmp_path)
    events = reopened.read_all()

    assert [event.event_id for event in events] == ["evt-1", "evt-2"]
    assert [event.seq for event in events] == [0, 1]
