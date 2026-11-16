"""PipeWire capture and queued local Whisper transcription with a Wayland overlay."""

import array
import math
import os
from pathlib import Path
import queue
import signal
import socket
import subprocess
import tempfile
import threading
import time
import wave

import gi

gi.require_version("Gtk", "3.0")
gi.require_version("Gdk", "3.0")
gi.require_version("GtkLayerShell", "0.1")
from gi.repository import Gdk, GLib, Gtk, GtkLayerShell, Pango  # noqa: E402


RATE = 16000
BLOCK = RATE // 10 * 2  # 100 ms of mono signed 16-bit PCM


def rms(pcm):
    samples = array.array("h", pcm)
    return math.sqrt(sum(x * x for x in samples) / max(1, len(samples)))


class Dictation:
    def __init__(self):
        self.stopping = threading.Event()
        self.chunks = queue.Queue()
        self.text = []
        self.error = None
        self.recorder = None
        self.started = time.monotonic()
        self.finished = False
        self.model = Path(os.environ.get(
            "DICTATE_MODEL",
            str(Path(os.environ.get("XDG_CACHE_HOME", str(Path.home() / ".cache")))
                / "berry-dictate/ggml-base.bin"),
        ))
        self.window = Gtk.Window(type=Gtk.WindowType.TOPLEVEL)
        self.window.set_default_size(490, -1)
        self.window.set_name("dictation")
        GtkLayerShell.init_for_window(self.window)
        GtkLayerShell.set_namespace(self.window, "berry-dictation")
        GtkLayerShell.set_layer(self.window, GtkLayerShell.Layer.OVERLAY)
        GtkLayerShell.set_anchor(self.window, GtkLayerShell.Edge.BOTTOM, True)
        GtkLayerShell.set_anchor(self.window, GtkLayerShell.Edge.LEFT, True)
        GtkLayerShell.set_margin(self.window, GtkLayerShell.Edge.BOTTOM, 24)
        GtkLayerShell.set_margin(self.window, GtkLayerShell.Edge.LEFT, 24)
        GtkLayerShell.set_keyboard_mode(self.window, GtkLayerShell.KeyboardMode.NONE)
        GtkLayerShell.set_exclusive_zone(self.window, -1)
        box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=9)
        box.set_border_width(16)
        self.heading = Gtk.Label(xalign=0)
        self.preview = Gtk.Label(label="Говорите…", xalign=0)
        self.preview.set_line_wrap(True)
        self.preview.set_line_wrap_mode(Pango.WrapMode.WORD_CHAR)
        self.preview.set_max_width_chars(62)
        box.pack_start(self.heading, False, False, 0)
        box.pack_start(self.preview, False, False, 0)
        self.window.add(box)
        css = Gtk.CssProvider()
        css.load_from_data(b"#dictation { background: #20242e; color: #eceff4; border: 2px solid #ffffff; border-radius: 12px; font: 14px sans-serif; }")
        Gtk.StyleContext.add_provider_for_screen(
            Gdk.Screen.get_default(), css, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
        signal.signal(signal.SIGUSR1, self.stop)
        signal.signal(signal.SIGTERM, self.cancel)
        signal.signal(signal.SIGINT, self.cancel)
        self.window.show_all()
        self.tick()
        GLib.timeout_add(250, self.tick)
        # Let systemctl start return only once the stop signal handler is ready.
        if address := os.environ.get("NOTIFY_SOCKET"):
            if address.startswith("@"):
                address = "\0" + address[1:]
            with socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM) as notifier:
                notifier.connect(address)
                notifier.sendall(b"READY=1")
        threading.Thread(target=self.run, daemon=True).start()

    def tick(self):
        if not self.finished:
            seconds = int(time.monotonic() - self.started)
            title = "◌ Завершаю расшифровку…" if self.stopping.is_set() else "● Идёт запись"
            self.heading.set_text(f"{title}  {seconds // 60:02}:{seconds % 60:02}  ·  Ctrl+Alt+R — стоп")
        return True

    def stop(self, *_):
        self.stopping.set()
        if self.recorder is not None and self.recorder.poll() is None:
            self.recorder.terminate()

    def cancel(self, *_):
        self.stop()
        Gtk.main_quit()

    def update_text(self):
        self.preview.set_text(" ".join(self.text)[-550:] or "Говорите…")
        return False

    def transcribe(self, directory):
        try:
            while (pcm := self.chunks.get()) is not None:
                path = Path(directory) / "chunk.wav"
                with wave.open(str(path), "wb") as wav:
                    wav.setnchannels(1)
                    wav.setsampwidth(2)
                    wav.setframerate(RATE)
                    wav.writeframes(pcm)
                result = subprocess.run([
                    "whisper-cli", "-m", str(self.model), "-f", str(path),
                    "-l", os.environ.get("DICTATE_LANGUAGE", "ru"),
                    "-ng", "-nt", "-sns", "-otxt", "-of", str(Path(directory) / "text"),
                ], capture_output=True, text=True, timeout=120)
                if result.returncode:
                    raise RuntimeError("Whisper: " + result.stderr[-400:])
                text = " ".join((Path(directory) / "text.txt").read_text().split())
                if text:
                    self.text.append(text)
                    GLib.idle_add(self.update_text)
        except Exception as exc:
            self.error = str(exc)
            self.stop()

    def capture(self):
        # Split at pauses, so inference runs while PipeWire keeps recording.
        pending = bytearray()
        silence = 0
        speech = 0
        while block := self.recorder.stdout.read(BLOCK):
            pending.extend(block)
            if rms(block) >= 180:
                speech += 1
                silence = 0
            else:
                silence += 1
            if (len(pending) >= RATE * 2 * 2 and silence >= 6) or len(pending) >= RATE * 2 * 12:
                if speech >= 3:
                    self.chunks.put(bytes(pending))
                # Retain a little silence as context for the next phrase.
                pending = bytearray(block if silence else b"")
                speech = 0
        if speech >= 2:
            self.chunks.put(bytes(pending))

    def run(self):
        try:
            if not self.model.is_file():
                raise RuntimeError(f"Нет модели Whisper: {self.model}")
            with tempfile.TemporaryDirectory(prefix="berry-dictation-") as directory:
                with tempfile.TemporaryFile() as log:
                    if self.stopping.is_set():
                        GLib.idle_add(self.complete)
                        return
                    self.recorder = subprocess.Popen([
                        "pw-record", "--raw", "--rate", str(RATE), "--channels", "1",
                        "--format", "s16", "-",
                    ], stdout=subprocess.PIPE, stderr=log)
                    worker = threading.Thread(target=self.transcribe, args=(directory,))
                    worker.start()
                    try:
                        self.capture()
                        code = self.recorder.wait()
                        if not self.stopping.is_set():
                            log.seek(0)
                            raise RuntimeError(f"Микрофон отключился ({code}): " + log.read().decode(errors="replace")[-250:])
                    finally:
                        self.stop()
                        self.recorder.wait(timeout=3)
                        self.recorder.stdout.close()
                        self.chunks.put(None)
                        worker.join()
                if self.error:
                    raise RuntimeError(self.error)
                if self.text:
                    subprocess.run(["wl-copy", "--type", "text/plain"],
                                   input=" ".join(self.text), text=True, check=True, timeout=5)
        except Exception as exc:
            self.error = str(exc)
            print(self.error, flush=True)
        GLib.idle_add(self.complete)

    def complete(self):
        self.finished = True
        if self.error:
            self.heading.set_text("Ошибка диктовки")
            self.preview.set_text(self.error)
        elif self.text:
            self.heading.set_text("✓ Текст скопирован · Ctrl+Shift+V — вставить")
            self.update_text()
        else:
            self.heading.set_text("Речь не распознана")
            self.preview.set_text("Буфер обмена не изменён.")
        GLib.timeout_add_seconds(6 if self.error else 2, Gtk.main_quit)
        return False


if __name__ == "__main__":
    os.umask(0o077)
    Dictation()
    Gtk.main()
