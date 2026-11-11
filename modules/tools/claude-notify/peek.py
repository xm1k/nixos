"""claude-peek: плашка в правом нижнем углу сообщает, какая сессия
Claude закончила работу (тёмная, полупрозрачная, оранжевая рамка, шрифт терминала).
Справа от плашки из-под её нижнего края пружинисто, с отдачей, выпрыгивает
пиксельный Claude в полный рост.

ЛКМ — перейти к окну и закрыть, ПКМ — просто закрыть.
Пока курсор над плашкой, она не уходит.

Работает как служба systemd --user `claude-peek` (выключить: claude-peek-off,
включить: claude-peek-on). Во время демонстрации экрана (niri msg casts)
уведомления не показываются.

claude-peek --demo — предпросмотр (15 с); --scale K (0.25.., по умолчанию 1
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
from gi.repository import GLib, Pango, PangoCairo  # noqa: E402

FONT = "JetBrainsMono Nerd Font"  # как в Alacritty

ORANGE = (0.843, 0.467, 0.341)  # #D77757 — цвет Claude Code
CREAM = (0.93, 0.91, 0.88)
DIM = (0.60, 0.58, 0.56)
BG = (0.08, 0.08, 0.09, 0.82)  # тёмный, немного прозрачный

# Тайминги (сек)
RISE_T = 0.15
STAY_T = 3.0
LEAVE_T = 0.25

BW = 380  # ширина плашки
R = 8  # радиус скругления
BORDER = 1.5
PAD = 14
SLIDE = 10  # на сколько пикселей выезжает снизу

# Пиксельный маскот Claude Code в полный рост: голова, руки, ноги.
SPRITE = [
    "...XXXXXXXXXXXX...",
    "...XXXXXXXXXXXX...",
    "...XXEXXXXXXEXX...",
    "...XXEXXXXXXEXX...",
    ".XXXXXXXXXXXXXXXX.",
    ".XXXXXXXXXXXXXXXX.",
    "...XXXXXXXXXXXX...",
    "...XXXXXXXXXXXX...",
    "....X.X....X.X....",
    "....X.X....X.X....",
]
P = 5  # размер пикселя маскота; высота = высоте плашки
INK = (0.08, 0.07, 0.07)
CW, CH = len(SPRITE[0]) * P, len(SPRITE) * P
GAP = 8  # между плашкой и маскотом
# Выдвижение Claude — затухающая пружина: 1 - e^(-DAMP·s)·cos(FREQ·s)
CLAUDE_DELAY = 0.06  # чуть позже плашки
SPRING_DAMP = 7.0
SPRING_FREQ = 17.0  # первый перелёт ~27% высоты, потом отдача вниз и успокоение
TOP = 16  # запас сверху под перелёт

BH = CH  # высота плашки
W = BW + GAP + CW
H = TOP + BH + SLIDE
CX0 = BW + GAP
BASE = TOP + BH  # линия, из-под которой выдвигается Claude


def clamp(x, a=0.0, b=1.0):
    return max(a, min(b, x))


def ease(x):
    return 1 - (1 - x) ** 3


class Scene:
    def __init__(self, project, detail):
        detail = detail.lstrip("✳✶✻✽✢·*⠀⠁⠂⠄⡀⢀⠠⠐⠈ \t")
        cr = cairo.Context(cairo.ImageSurface(cairo.FORMAT_ARGB32, 1, 1))
        max_w = BW - 2 * PAD - 2 * BORDER
        self.lines = [self._layout(cr, f"{project or 'claude'} — готово", 13, True, max_w, CREAM, mark=True)]
        if detail:
            self.lines.append(self._layout(cr, detail, 11.5, False, max_w, DIM))

    @staticmethod
    def _layout(cr, text, px, bold, width, color, mark=False):
        layout = PangoCairo.create_layout(cr)
        fo = cairo.FontOptions()
        fo.set_antialias(cairo.ANTIALIAS_GRAY)
        fo.set_hint_style(cairo.HINT_STYLE_SLIGHT)
        PangoCairo.context_set_font_options(layout.get_context(), fo)
        fd = Pango.FontDescription(FONT)
        fd.set_absolute_size(px * Pango.SCALE)
        if bold:
            fd.set_weight(Pango.Weight.BOLD)
        layout.set_font_description(fd)
        if mark:  # оранжевая звёздочка Claude перед заголовком
            star = '<span foreground="#D77757">✻</span> '
            layout.set_markup(star + GLib.markup_escape_text(text, -1), -1)
        else:
            layout.set_text(text, -1)
        layout.set_width(int(width * Pango.SCALE))
        layout.set_ellipsize(Pango.EllipsizeMode.END)
        return layout, color

    def input_rects(self):
        return [(0, TOP, W, BH)]

    def draw(self, cr, t, leave_t=None):
        cr.set_operator(cairo.OPERATOR_SOURCE)
        cr.set_source_rgba(0, 0, 0, 0)
        cr.paint()
        cr.set_operator(cairo.OPERATOR_OVER)

        k = ease(clamp(t / RISE_T))
        if leave_t is not None:
            k *= 1 - ease(clamp(leave_t / LEAVE_T))
        if k <= 0:
            return
        cr.push_group()
        cr.translate(0, TOP + round((1 - k) * SLIDE))
        self._draw_banner(cr)
        cr.pop_group_to_source()
        cr.paint_with_alpha(k)
        self._draw_claude(cr, self._rise(t, leave_t))

    @staticmethod
    def _rise(t, leave_t):
        """Насколько Claude выдвинут (доля высоты): пружина с перелётом и отдачей;
        при уходе — короткий подскок и нырок вниз (ease-in-back)."""
        s = t - CLAUDE_DELAY
        if s <= 0:
            return 0.0
        p = max(0.0, 1 - math.exp(-SPRING_DAMP * s) * math.cos(SPRING_FREQ * s))
        if leave_t is not None:
            q = clamp(leave_t / LEAVE_T)
            p *= 1 - q * q * (2.7 * q - 1.7)
        return p

    @staticmethod
    def _draw_claude(cr, p):
        if p <= 0:
            return
        cr.save()
        cr.set_antialias(cairo.ANTIALIAS_NONE)
        cr.rectangle(CX0, 0, CW, BASE)  # ниже линии не видно — «прячется»
        cr.clip()
        top = BASE - round(CH * p)
        for r, line in enumerate(SPRITE):
            for c, ch in enumerate(line):
                if ch == ".":
                    continue
                cr.set_source_rgb(*(INK if ch == "E" else ORANGE))
                cr.rectangle(CX0 + c * P, top + r * P, P, P)
                cr.fill()
        cr.restore()

    @staticmethod
    def _rounded(cr, x, y, w, h, r):
        cr.new_sub_path()
        cr.arc(x + w - r, y + r, r, -math.pi / 2, 0)
        cr.arc(x + w - r, y + h - r, r, 0, math.pi / 2)
        cr.arc(x + r, y + h - r, r, math.pi / 2, math.pi)
        cr.arc(x + r, y + r, r, math.pi, 3 * math.pi / 2)
        cr.close_path()

    def _draw_banner(self, cr):
        h_box = BH
        b = BORDER / 2
        self._rounded(cr, b, b, BW - 2 * b, h_box - 2 * b, R)
        cr.set_source_rgba(*BG)
        cr.fill_preserve()
        cr.set_source_rgb(*ORANGE)
        cr.set_line_width(BORDER)
        cr.stroke()

        text_h = sum(l.get_pixel_size()[1] for l, _ in self.lines) + 2 * (len(self.lines) - 1)
        y = (h_box - text_h) / 2
        for layout, color in self.lines:
            cr.move_to(PAD, y)
            cr.set_source_rgb(*color)
            PangoCairo.show_layout(cr, layout)
            y += layout.get_pixel_size()[1] + 2


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
    GtkLayerShell.set_margin(win, GtkLayerShell.Edge.RIGHT, 24)
    GtkLayerShell.set_margin(win, GtkLayerShell.Edge.BOTTOM, 24 + int(slot * (BH + 8) * k))
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
        cr.scale(k, k)
        scene.draw(cr, now - state["t0"], lt)
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
    stay = CLAUDE_DELAY + 0.7
    total = stay + LEAVE_T + 0.1
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
        default=float(os.environ.get("CLAUDE_PEEK_SCALE", 1.0)),
        help="масштаб, например 0.75 или 2 (по умолчанию $CLAUDE_PEEK_SCALE или 1)",
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
