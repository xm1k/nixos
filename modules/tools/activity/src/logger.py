"""activity-niri-log: пишет niri event-stream в ~/.local/share/activity/niri-YYYY-MM-DD.jsonl.

Каждая строка: {"t": <unix time float>, "ev": "<EventName>", "d": {...}}.
Шумные события (геометрия окон) отбрасываются, остальное сохраняется как есть.
При обрыве (перезапуск niri) — переподключение.
"""
import json
import os
import signal
import subprocess
import sys
import time
from pathlib import Path

NIRI = os.environ.get("ACTIVITY_NIRI", "niri")
DIR = Path(os.environ.get("XDG_DATA_HOME", Path.home() / ".local/share")) / "activity"
SKIP = {"WindowLayoutsChanged"}


def out_file():
    return DIR / time.strftime("niri-%Y-%m-%d.jsonl")


def run():
    DIR.mkdir(parents=True, exist_ok=True)
    proc = subprocess.Popen([NIRI, "msg", "--json", "event-stream"], stdout=subprocess.PIPE, text=True)
    path, fh = None, None
    try:
        for line in proc.stdout:
            try:
                obj = json.loads(line)
            except ValueError:
                continue
            ev, d = next(iter(obj.items()))
            if ev in SKIP:
                continue
            if ev == "WindowOpenedOrChanged":
                w = d.get("window", {})
                w.pop("layout", None)
            elif ev == "WindowsChanged":
                for w in d.get("windows", []):
                    w.pop("layout", None)
            p = out_file()
            if p != path:
                if fh:
                    fh.close()
                path, fh = p, open(p, "a", buffering=1)
            fh.write(json.dumps({"t": round(time.time(), 3), "ev": ev, "d": d}, ensure_ascii=False) + "\n")
    finally:
        if fh:
            fh.write(json.dumps({"t": round(time.time(), 3), "ev": "LoggerStopped", "d": {}}) + "\n")
            fh.close()
        proc.kill()
    return proc.wait()


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    sys.exit(run())
