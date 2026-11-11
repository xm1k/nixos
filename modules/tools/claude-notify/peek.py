"""claude-peek: пиксельный Claude выглядывает из-за края экрана и
сообщает, какая сессия закончила работу.

ЛКМ — перейти к окну и закрыть, ПКМ — просто закрыть.
Пока курсор над персонажем/облачком, он не уходит.

Работает как служба systemd --user `claude-peek` (выключить: claude-peek-off,
включить: claude-peek-on). Во время демонстрации экрана (niri msg casts)
уведомления не показываются.

claude-peek --demo — предпросмотр (15 с); --scale K (0.25.., по умолчанию 1.5
или $CLAUDE_PEEK_SCALE), --stay SEC.
--frames DIR — отрендерить анимацию в PNG-кадры (для превью), без окна.
"""

import argparse
import json
import sys
import math
import os
import signal
import socket
import subprocess
import time

import cairo
import gi

gi.require_version("Pango", "1.0")
gi.require_version("PangoCairo", "1.0")
from gi.repository import Pango, PangoCairo  # noqa: E402

FONT = "Terminus (TTF)"

ORANGE = (0.843, 0.467, 0.341)  # #D77757 — цвет маскота Claude Code
INK = (0.08, 0.07, 0.07)
CREAM = (0.96, 0.93, 0.88)
DIM = (0.62, 0.60, 0.58)
BUBBLE_BG = (0.10, 0.10, 0.12)
SHADOW = (0.0, 0.0, 0.0, 0.45)

# Тайминги (сек)
RISE_T = 0.1
STAY_T = 3.0
LEAVE_T = 0.1

P = 6  # размер пикселя персонажа
B = 2  # размер пикселя облачка

# Спрайт маскота; над краем экрана видны строки 0-5 (руки держатся за край).
SPRITE = [
    "...XXXXXXXXXXXX...",
    "...XXXXXXXXXXXX...",
    "...XXEXXXXXXEXX...",
    "...XXEXXXXXXEXX...",
    ".XXXXXXXXXXXXXXXX.",
    ".XXXXXXXXXXXXXXXX.",
]
COLS = len(SPRITE[0])
ROWS = len(SPRITE)
CW = COLS * P
W, H = 440, 72
CX0 = W - CW - 2


def clamp(x, a=0.0, b=1.0):
    return max(a, min(b, x))


def snap(v, step):
    return round(v / step) * step


class Scene:
    def __init__(self, project, detail):
        detail = detail.lstrip("✳✶✻✽✢·*⠀⠁⠂⠄⡀⢀⠠⠐⠈ \t")
        cr = cairo.Context(cairo.ImageSurface(cairo.FORMAT_ARGB32, 1, 1))
        right = CX0 - 4 * B
        pad = 4 * B
        max_w = right - 2 * pad - 2 * B
        self.lines = [self._layout(cr, f"{project or 'claude'} — готово", 16, True, max_w, CREAM)]
        if detail:
            self.lines.append(self._layout(cr, detail, 14, False, max_w, DIM))
        h = sum(l.get_pixel_size()[1] for l, _ in self.lines)
        inner = max(l.get_pixel_size()[0] for l, _ in self.lines)
        self.bw = snap(inner + 2 * pad, B)
        self.bh = snap(h + 2 * pad - 2, B)
        self.bx = snap(right - self.bw, B)
        self.by = H - self.bh - 2 * B
        self.pad = pad

    @staticmethod
    def _layout(cr, text, px, bold, width, color):
        layout = PangoCairo.create_layout(cr)
        fo = cairo.FontOptions()
        fo.set_antialias(cairo.ANTIALIAS_NONE)
        fo.set_hint_style(cairo.HINT_STYLE_NONE)  # с хинтингом Terminus 14px рвётся
        fo.set_hint_metrics(cairo.HINT_METRICS_ON)
        PangoCairo.context_set_font_options(layout.get_context(), fo)
        fd = Pango.FontDescription(FONT)
        fd.set_absolute_size(px * Pango.SCALE)
        if bold:
            fd.set_weight(Pango.Weight.BOLD)
        layout.set_font_description(fd)
        layout.set_text(text, -1)
        layout.set_width(int(width * Pango.SCALE))
        layout.set_ellipsize(Pango.EllipsizeMode.END)
        return layout, color

    def input_rects(self):
        return [(self.bx, self.by, W - self.bx, H - self.by)]

    def draw(self, cr, t, leave_t=None):
        cr.set_antialias(cairo.ANTIALIAS_NONE)
        cr.set_operator(cairo.OPERATOR_SOURCE)
        cr.set_source_rgba(0, 0, 0, 0)
        cr.paint()
        cr.set_operator(cairo.OPERATOR_OVER)

        k = clamp(t / RISE_T)
        if leave_t is not None:
            k *= 1 - clamp(leave_t / LEAVE_T)
        if k >= 1:
            self._draw_bubble(cr)
        rows_up = math.floor(ROWS * k)
        if rows_up > 0:
            self._draw_claude(cr, H - rows_up * P)

    def _draw_claude(self, cr, top):
        for r, line in enumerate(SPRITE):
            for c, ch in enumerate(line):
                if ch == ".":
                    continue
                cr.set_source_rgb(*(INK if ch == "E" else ORANGE))
                cr.rectangle(CX0 + c * P, top + r * P, P, P)
                cr.fill()

    def _box(self, cr, x, y, w, h, color):
        cr.set_source_rgba(*color)
        cr.rectangle(x + B, y, w - 2 * B, h)
        cr.rectangle(x, y + B, B, h - 2 * B)
        cr.rectangle(x + w - B, y + B, B, h - 2 * B)
        cr.fill()

    def _draw_bubble(self, cr):
        x, y, w, h = self.bx, self.by, self.bw, self.bh
        self._box(cr, x + B, y + B, w, h, SHADOW)
        self._box(cr, x, y, w, h, (*ORANGE, 1))
        self._box(cr, x + B, y + B, w - 2 * B, h - 2 * B, (*BUBBLE_BG, 1))
        # хвостик
        ty = y + h - 6 * B - 3 * P
        cr.set_source_rgb(*ORANGE)
        cr.rectangle(x + w, ty, 2 * B, 3 * B)
        cr.rectangle(x + w + 2 * B, ty + B, B, B)
        cr.fill()
        cr.set_source_rgb(*BUBBLE_BG)
        cr.rectangle(x + w - B, ty + B, 2 * B, B)
        cr.fill()
        ly = y + self.pad - 2
        for layout, color in self.lines:
            cr.move_to(x + self.pad, ly)
            cr.set_source_rgb(*color)
            PangoCairo.show_layout(cr, layout)
            ly += layout.get_pixel_size()[1]


# ---------------------------------------------------------------- слоты
def take_slot():
    """Несколько одновременных уведомлений встают рядом друг с другом."""
    d = os.path.join(os.environ.get("XDG_RUNTIME_DIR", "/tmp"), "claude-peek")
    os.makedirs(d, exist_ok=True)
    for i in range(8):
        p = os.path.join(d, f"slot{i}")
        for _ in range(2):
            try:
                fd = os.open(p, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
                os.write(fd, str(os.getpid()).encode())
                os.close(fd)
                return i, p
            except FileExistsError:
                try:
                    with open(p) as f:
                        os.kill(int(f.read().strip()), 0)
                    break  # слот живой
                except (ValueError, ProcessLookupError, FileNotFoundError):
                    try:
                        os.unlink(p)  # протухший слот
                    except FileNotFoundError:
                        pass
    return 0, None


# ---------------------------------------------------------------- окно
def run_window(args, scene, slot):
    gi.require_version("Gtk", "3.0")
    gi.require_version("Gdk", "3.0")
    gi.require_version("GtkLayerShell", "0.1")
    from gi.repository import Gdk, GLib, Gtk, GtkLayerShell

    state = dict(t0=time.monotonic(), leave_at=None, hover=False)
    state["deadline"] = state["t0"] + args.stay
    k = max(0.25, args.scale)

    win = Gtk.Window(title="claude-peek")
    win.set_app_paintable(True)
    win.set_decorated(False)
    visual = win.get_screen().get_rgba_visual()
    if visual is not None:
        win.set_visual(visual)

    GtkLayerShell.init_for_window(win)
    GtkLayerShell.set_namespace(win, "claude-peek")
    GtkLayerShell.set_layer(win, GtkLayerShell.Layer.OVERLAY)
    GtkLayerShell.set_anchor(win, GtkLayerShell.Edge.BOTTOM, True)
    GtkLayerShell.set_anchor(win, GtkLayerShell.Edge.RIGHT, True)
    GtkLayerShell.set_margin(win, GtkLayerShell.Edge.RIGHT, 24 + int(slot * (W + 8) * k))
    GtkLayerShell.set_exclusive_zone(win, -1)
    GtkLayerShell.set_keyboard_mode(win, GtkLayerShell.KeyboardMode.NONE)

    area = Gtk.DrawingArea()
    area.set_size_request(round(W * k), round(H * k))
    win.add(area)

    def leave():
        if state["leave_at"] is None:
            state["leave_at"] = time.monotonic()

    def on_draw(_w, cr):
        now = time.monotonic()
        lt = None if state["leave_at"] is None else now - state["leave_at"]
        if k == 1:
            scene.draw(cr, now - state["t0"], lt)
            return False
        # масштаб: рисуем в 1x и растягиваем (вверх — без сглаживания, чтобы пиксели
        # оставались квадратными; вниз — со сглаживанием, иначе пиксели теряются)
        surf = cairo.ImageSurface(cairo.FORMAT_ARGB32, W, H)
        scene.draw(cairo.Context(surf), now - state["t0"], lt)
        cr.set_operator(cairo.OPERATOR_SOURCE)
        cr.scale(k, k)
        cr.set_source_surface(surf, 0, 0)
        cr.get_source().set_filter(cairo.FILTER_NEAREST if k >= 1 else cairo.FILTER_GOOD)
        cr.paint()
        return False

    def on_click(_w, ev):
        if ev.button == 1 and args.win_id:
            subprocess.Popen(
                ["niri", "msg", "action", "focus-window", "--id", str(args.win_id)],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        leave()
        return True

    def on_enter(*_):
        state["hover"] = True

    def on_leave(*_):
        state["hover"] = False
        state["deadline"] = max(state["deadline"], time.monotonic() + 2.0)

    def on_realize(*_):
        region = cairo.Region([cairo.RectangleInt(*(round(v * k) for v in r)) for r in scene.input_rects()])
        win.input_shape_combine_region(region)
        gw = win.get_window()
        if gw is not None:
            gw.set_cursor(Gdk.Cursor.new_from_name(win.get_display(), "pointer"))

    def tick():
        now = time.monotonic()
        if state["leave_at"] is None and not state["hover"] and now > state["deadline"]:
            leave()
        if state["leave_at"] is not None and now - state["leave_at"] > LEAVE_T + 0.05:
            win.destroy()
            return False
        area.queue_draw()
        return True

    win.add_events(
        Gdk.EventMask.BUTTON_PRESS_MASK
        | Gdk.EventMask.ENTER_NOTIFY_MASK
        | Gdk.EventMask.LEAVE_NOTIFY_MASK
    )
    area.connect("draw", on_draw)
    win.connect("button-press-event", on_click)
    win.connect("enter-notify-event", on_enter)
    win.connect("leave-notify-event", on_leave)
    win.connect("realize", on_realize)
    win.connect("destroy", Gtk.main_quit)
    GLib.timeout_add(16, tick)
    win.show_all()
    Gtk.main()


def render_frames(scene, out, fps):
    """Кадры: подъём, печать, ожидание, уход."""
    os.makedirs(out, exist_ok=True)
    stay, total = 0.6, 0.6 + LEAVE_T + 0.1
    for i in range(int(total * fps) + 1):
        t = i / fps
        surf = cairo.ImageSurface(cairo.FORMAT_ARGB32, W, H)
        cr = cairo.Context(surf)
        scene.draw(cr, t, None if t < stay else t - stay)
        surf.write_to_png(os.path.join(out, f"f{i:04d}.png"))


# ---------------------------------------------------------------- служба
def socket_path():
    return os.path.join(os.environ.get("XDG_RUNTIME_DIR", "/tmp"), "claude-peek.sock")


def screen_is_shared():
    """niri сам знает про активные трансляции экрана (портал, OBS и т.п.)."""
    try:
        out = subprocess.run(
            ["niri", "msg", "--json", "casts"], capture_output=True, text=True, timeout=2
        )
        return out.returncode == 0 and json.loads(out.stdout or "[]") != []
    except (OSError, ValueError, subprocess.TimeoutExpired):
        return False


def run_daemon():
    """Принимает события от хука и показывает уведомления.
    Остановлена служба — уведомлений нет; идёт демонстрация экрана — тоже нет."""
    path = socket_path()
    try:
        os.unlink(path)
    except FileNotFoundError:
        pass
    srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    srv.bind(path)
    os.chmod(path, 0o600)
    srv.listen(8)
    signal.signal(signal.SIGCHLD, signal.SIG_IGN)  # детей не ждём, зомби не копим

    def stop(*_):
        try:
            os.unlink(path)
        except FileNotFoundError:
            pass
        sys.exit(0)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    print(f"claude-peek: слушаю {path}", flush=True)
    while True:
        conn, _ = srv.accept()
        with conn:
            conn.settimeout(2)
            try:
                data = b""
                while chunk := conn.recv(4096):
                    data += chunk
                ev = json.loads(data or b"{}")
            except (OSError, ValueError):
                continue
        if screen_is_shared():
            print(f"claude-peek: идёт демонстрация экрана, пропускаю {ev.get('project')}", flush=True)
            continue
        subprocess.Popen(
            [sys.executable, os.path.abspath(__file__),
             "--project", str(ev.get("project", "")),
             "--detail", str(ev.get("detail", "")),
             "--win-id", str(ev.get("win_id", ""))],
            stdin=subprocess.DEVNULL,
        )


def send_notify(args):
    ev = dict(project=args.project, detail=args.detail, win_id=args.win_id)
    try:
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as c:
            c.settimeout(1)
            c.connect(socket_path())
            c.sendall(json.dumps(ev).encode())
    except OSError:
        pass  # служба выключена — значит, молчим


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--project", default="")
    ap.add_argument("--detail", default="")
    ap.add_argument("--win-id", default="")
    ap.add_argument("--daemon", action="store_true", help="запустить службу (для systemd)")
    ap.add_argument("--notify", action="store_true", help="отправить событие службе (для хука)")
    ap.add_argument("--frames", help="отрендерить кадры в каталог и выйти")
    ap.add_argument("--fps", type=int, default=30)
    ap.add_argument("--stay", type=float, default=STAY_T, help="сколько секунд показывать")
    ap.add_argument(
        "--scale",
        type=float,
        default=float(os.environ.get("CLAUDE_PEEK_SCALE", 1.5)),
        help="масштаб, например 0.75 или 2 (по умолчанию $CLAUDE_PEEK_SCALE или 1.5)",
    )
    ap.add_argument(
        "--demo",
        action="store_true",
        help="предпросмотр: пример данных, 15 с (можно переопределить флагами)",
    )
    args = ap.parse_args()
    if args.demo:
        args.project = args.project or os.path.basename(os.getcwd()) or "nixos"
        args.detail = args.detail or "Заголовок окна сессии (задача Claude)"
        if args.stay == STAY_T:
            args.stay = 15.0

    if args.daemon:
        run_daemon()
        return
    if args.notify:
        send_notify(args)
        return
    if args.frames:
        render_frames(Scene(args.project, args.detail), args.frames, args.fps)
        return

    slot, slot_path = take_slot()
    try:
        run_window(args, Scene(args.project, args.detail), slot)
    finally:
        if slot_path:
            try:
                os.unlink(slot_path)
            except FileNotFoundError:
                pass


if __name__ == "__main__":
    main()
