"""activity-report: статистика по командам shell (atuin) и событиям niri.

  activity-report              # последние 7 дней
  activity-report -d 1         # за сутки
  activity-report -s shell     # только shell (или niri)
  activity-report -n 30        # длиннее топы

Источники:
  ~/.local/share/atuin/history.db           — команды: время, длительность, exit, cwd, сессия
  ~/.local/share/activity/niri-*.jsonl      — события niri (activity-niri-log)
  ~/.local/share/activity/idle.jsonl        — idle/lock/sleep (hypridle → activity-mark)
"""
import argparse
import glob
import json
import os
import re
import shlex
import sqlite3
import statistics
import time
from collections import Counter, defaultdict
from pathlib import Path

DATA = Path(os.environ.get("XDG_DATA_HOME", Path.home() / ".local/share"))
ACT = DATA / "activity"
ATUIN_DB = DATA / "atuin" / "history.db"

# Команды-«сессии»: их длительность — работа внутри, а не ожидание.
INTERACTIVE = {
    "ssh", "mosh", "nvim", "vim", "vi", "nano", "claude", "kilo", "less", "more", "man",
    "htop", "btop", "top", "k9s", "zj", "zj-here", "shpool", "bash", "sh", "zsh", "fish",
    "python", "python3", "ipython", "psql", "mysql", "sqlite3", "watch", "tmux", "zellij",
    "work-vpn", "fzf", "yazi", "lazygit", "tig", "nix-shell", "bat", "journalctl", "tail",
}
INTERACTIVE_PREFIX = ("nix develop", "kubectl exec", "k0s kubectl exec", "podman exec", "podman run -it",
                      "docker exec", "kubectl logs -f", "git log", "git diff", "git show")
INTERACTIVE_AFTER = 15 * 60          # дольше 15 минут — точно не «ожидание»
PREP_MAX = 120                       # пауза перед командой до 2 мин считается набором/обдумыванием
SEQ_GAP = 300                        # команды ближе 5 мин — одна цепочка
WRAPPERS = {"sudo", "doas", "time", "nohup", "env", "command", "exec", "builtin"}
NOISE = {"clear", "ls", "ll", "la", "exit", "pwd"}


def fmt(sec):
    sec = int(round(sec))
    if sec < 60:
        return f"{sec}s"
    m, s = divmod(sec, 60)
    if m < 60:
        return f"{m}m{s:02d}s"
    h, m = divmod(m, 60)
    return f"{h}h{m:02d}m"


def table(rows, headers, aligns=None):
    rows = [[str(c) for c in r] for r in rows]
    if not rows:
        print("  (нет данных)")
        return
    widths = [max(len(h), *(len(r[i]) for r in rows)) for i, h in enumerate(headers)]
    aligns = aligns or ["<"] + [">"] * (len(headers) - 1)
    line = lambda r: "  " + "  ".join(f"{c:{a}{w}}" for c, a, w in zip(r, aligns, widths))
    print(line(headers))
    print("  " + "  ".join("─" * w for w in widths))
    for r in rows:
        print(line(r))


def header(title):
    print(f"\n━━ {title} " + "━" * max(0, 70 - len(title)))


def clip(s, n=70):
    s = " ".join(s.split())
    return s if len(s) <= n else s[: n - 1] + "…"


# ─────────────────────────────── shell ───────────────────────────────

def tokens(cmd):
    try:
        t = shlex.split(cmd, comments=False)
    except ValueError:
        t = cmd.split()
    while t and (re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", t[0]) or t[0] in WRAPPERS):
        t = t[1:]
    return t


def tool_key(cmd):
    t = tokens(cmd)
    if not t:
        return cmd.strip()
    base = os.path.basename(t[0]) if "/" in t[0] and not t[0].startswith("./") else t[0]
    if len(t) > 1 and re.fullmatch(r"[a-z][a-z0-9-]{0,15}", t[1]):
        return f"{base} {t[1]}"
    return base


SUBS = [
    (re.compile(r"https?://\S+"), "<URL>"),
    (re.compile(r"\b(\w[\w.-]*@)?\d{1,3}(\.\d{1,3}){3}(/\d+)?(:\d+)?\b"), "<IP>"),
    (re.compile(r"\b[A-Z][A-Z0-9]+-\d+\b"), "<ISSUE>"),
    (re.compile(r"\b[0-9a-f]{7,40}\b"), "<HASH>"),
    (re.compile(r"(?<![\w.-])\d+(?![\w.-])"), "<N>"),
]


def pattern(cmd):
    s = " ".join(cmd.split())
    for rx, rep in SUBS:
        s = rx.sub(rep, s)
    return s


def is_interactive(cmd, dur):
    t = tokens(cmd)
    if not t:
        return False
    if os.path.basename(t[0]) in INTERACTIVE or t[0].endswith("run.sh"):
        return True
    joined = " ".join(t)
    if any(joined.startswith(p) for p in INTERACTIVE_PREFIX):
        return True
    return dur is not None and dur > INTERACTIVE_AFTER


def load_shell(since):
    if not ATUIN_DB.exists():
        return []
    con = sqlite3.connect(f"file:{ATUIN_DB}?mode=ro", uri=True)
    q = """SELECT timestamp, duration, exit, command, cwd, session FROM history
           WHERE timestamp >= ? AND deleted_at IS NULL ORDER BY session, timestamp"""
    rows = []
    for ts, dur, ex, cmd, cwd, sess in con.execute(q, (int(since * 1e9),)):
        d = dur / 1e9 if dur is not None and dur >= 0 else None
        rows.append({"t": ts / 1e9, "dur": d, "exit": ex, "cmd": cmd, "cwd": cwd, "sess": sess})
    con.close()
    return rows


def report_shell(rows, top):
    header(f"SHELL — {len(rows)} команд")
    if not rows:
        print("  История atuin пуста: данные начнут копиться после rebuild и нового терминала.")
        return

    by_sess = defaultdict(list)
    for r in rows:
        by_sess[r["sess"]].append(r)
    for lst in by_sess.values():
        prev = None
        for r in lst:
            r["prep"] = None
            r["inter"] = is_interactive(r["cmd"], r["dur"])
            if prev is not None and prev["dur"] is not None:
                gap = r["t"] - (prev["t"] + prev["dur"])
                if 0 <= gap <= PREP_MAX:
                    r["prep"] = gap
            prev = r

    agg = defaultdict(lambda: {"n": 0, "wait": 0.0, "prep": 0.0, "fail": 0, "inter": 0.0})
    for r in rows:
        a = agg[tool_key(r["cmd"])]
        a["n"] += 1
        a["prep"] += r["prep"] or 0
        if r["exit"] not in (0, None, -1):
            a["fail"] += 1
        if r["dur"] is not None:
            a["inter" if r["inter"] else "wait"] += r["dur"]

    total_wait = sum(a["wait"] for a in agg.values())
    total_prep = sum(a["prep"] for a in agg.values())
    total_inter = sum(a["inter"] for a in agg.values())
    days = len({time.strftime("%F", time.localtime(r["t"])) for r in rows})
    print(f"  Дней с активностью: {days}.  Ожидание выполнения: {fmt(total_wait)}.  "
          f"Набор/обдумывание: {fmt(total_prep)}.  Внутри интерактивных: {fmt(total_inter)}.")

    print("\n  Команды по суммарной цене (ожидание + набор). Это кандидаты на скрипт/алиас:")
    items = sorted(agg.items(), key=lambda kv: -(kv[1]["wait"] + kv[1]["prep"]))
    table([[clip(k, 34), a["n"], fmt(a["wait"] + a["prep"]), fmt(a["wait"]), fmt(a["prep"]),
            fmt(a["wait"] / a["n"]), a["fail"] or ""] for k, a in items[:top]],
          ["команда", "раз", "цена", "ожидание", "набор", "ср.ожид", "fail"])

    print("\n  Самые частые команды целиком (аргументы обобщены):")
    pc = Counter(pattern(r["cmd"]) for r in rows if tool_key(r["cmd"]) not in NOISE)
    table([[clip(p, 60), n] for p, n in pc.most_common(top)], ["шаблон", "раз"])

    print("\n  Самые долгие ожидания (одиночные запуски):")
    slow = sorted((r for r in rows if r["dur"] and not r["inter"]), key=lambda r: -r["dur"])[:min(top, 10)]
    table([[clip(r["cmd"], 60), fmt(r["dur"]), time.strftime("%a %H:%M", time.localtime(r["t"]))] for r in slow],
          ["команда", "длит.", "когда"])

    seq2, seq3 = Counter(), Counter()
    for lst in by_sess.values():
        chain = []
        for r in lst:
            k = tool_key(r["cmd"])
            if k in NOISE:
                continue
            if chain and r["t"] - chain[-1][0] > SEQ_GAP:
                chain = []
            chain.append((r["t"] + (r["dur"] or 0), k))
            ks = [c[1] for c in chain]
            if len(ks) >= 2 and ks[-1] != ks[-2]:
                seq2[(ks[-2], ks[-1])] += 1
            if len(ks) >= 3 and len(set(ks[-3:])) > 1:
                seq3[tuple(ks[-3:])] += 1
    print("\n  Повторяющиеся цепочки (одна сессия, паузы < 5 мин). Кандидаты на одну команду:")
    rows3 = [[" → ".join(s), n] for s, n in seq3.most_common(top) if n >= 3]
    rows2 = [[" → ".join(s), n] for s, n in seq2.most_common(top) if n >= 3]
    table((rows3 + rows2)[:top], ["цепочка", "раз"])

    retry = Counter()
    for lst in by_sess.values():
        for a, b in zip(lst, lst[1:]):
            if a["exit"] not in (0, None, -1) and a["dur"] is not None and b["t"] - a["t"] - a["dur"] < 120 \
                    and tokens(a["cmd"])[:1] == tokens(b["cmd"])[:1]:
                retry[tool_key(a["cmd"])] += 1
    nf = Counter(tokens(r["cmd"])[0] for r in rows if r["exit"] == 127 and tokens(r["cmd"]))
    print("\n  Упало → сразу перезапустил с правкой (непонятный синтаксис / забытый флаг):")
    table([[k, n] for k, n in retry.most_common(min(top, 10))], ["команда", "раз"])
    if nf:
        print("\n  command not found (опечатки — кандидаты на алиас):")
        table([[k, n] for k, n in nf.most_common(10)], ["что набрал", "раз"])

    print("\n  Где работаешь (каталоги):")
    home = str(Path.home())
    cw = Counter((r["cwd"] or "?").replace(home, "~", 1) for r in rows)
    table([[clip(c, 50), n] for c, n in cw.most_common(min(top, 10))], ["каталог", "команд"])


# ─────────────────────────────── niri ───────────────────────────────

BROWSER_SUFFIX = re.compile(r"\s+[—–-]\s+(Zen Browser|Mozilla Firefox|Zen|Firefox|Chromium|Google Chrome)$")


def norm_title(app, title):
    title = BROWSER_SUFFIX.sub("", title or "")
    return clip(title, 60) or "(без заголовка)"


def load_jsonl(pattern_, since, until):
    out = []
    for f in sorted(glob.glob(str(ACT / pattern_))):
        with open(f) as fh:
            for line in fh:
                try:
                    o = json.loads(line)
                except ValueError:
                    continue
                if since - 86400 <= o.get("t", 0) <= until:
                    out.append(o)
    return out


def idle_intervals(marks, until):
    """[(start, end)] — периоды, когда человека нет за компьютером."""
    res, start = [], None
    for m in sorted(marks, key=lambda m: m["t"]):
        if m["ev"] in ("idle", "lock", "sleep"):
            s = m["t"] - m.get("back", 0)
            start = s if start is None else min(start, s)
        elif m["ev"] in ("active", "unlock", "wake") and start is not None:
            res.append((start, m["t"]))
            start = None
    if start is not None:
        res.append((start, until))
    return res


def subtract(a, b, idle):
    """Длительность [a, b) минус idle-периоды."""
    d = b - a
    for s, e in idle:
        lo, hi = max(a, s), min(b, e)
        if hi > lo:
            d -= hi - lo
    return max(d, 0)


def report_niri(since, until, top):
    events = load_jsonl("niri-*.jsonl", since, until)
    marks = load_jsonl("idle.jsonl", since, until)
    header(f"NIRI — {len(events)} событий")
    if not events:
        print("  Лог пуст: проверь `systemctl --user status activity-niri-log`.")
        return
    idle = idle_intervals(marks, until)

    windows, focused = {}, None
    seg_start, seg_key = None, None
    segs = []                                 # (start, end, active_sec, app, title)
    ws_switch = overview = layout_sw = 0
    online = False

    def close(t):
        nonlocal seg_start
        if seg_start is not None and seg_key is not None:
            a, b = max(seg_start, since), min(t, until)
            if b > a:
                segs.append((a, b, subtract(a, b, idle), *seg_key))
        seg_start = t

    def refocus(t):
        nonlocal seg_key
        w = windows.get(focused)
        key = (w["app_id"] or "?", norm_title(w["app_id"], w["title"])) if w else None
        if key != seg_key:
            close(t)
            seg_key = key

    for e in events:
        t, ev, d = e["t"], e["ev"], e["d"]
        if ev == "LoggerStopped":
            close(t)
            seg_key, online = None, False
            continue
        if ev == "WindowsChanged":
            if not online:
                close(t)
                seg_key, online = None, True
            windows = {w["id"]: w for w in d["windows"]}
            focused = next((w["id"] for w in d["windows"] if w.get("is_focused")), None)
            refocus(t)
        elif ev == "WindowOpenedOrChanged":
            w = d["window"]
            windows[w["id"]] = w
            if w.get("is_focused"):
                focused = w["id"]
            refocus(t)
        elif ev == "WindowClosed":
            windows.pop(d["id"], None)
            if focused == d["id"]:
                focused = None
            refocus(t)
        elif ev == "WindowFocusChanged":
            focused = d.get("id")
            refocus(t)
        elif ev == "WorkspaceActivated" and d.get("focused") and since <= t <= until:
            ws_switch += 1
        elif ev == "OverviewOpenedOrClosed" and d.get("is_open") and since <= t <= until:
            overview += 1
        elif ev == "KeyboardLayoutSwitched" and since <= t <= until:
            layout_sw += 1
    if online:
        close(min(time.time(), until))

    # Склеиваем соседние сегменты одного окна (меняется только заголовок) в «визиты» приложения.
    visits = []                               # [app, start, end, active]
    for s, e_, act, app, _ in segs:
        if visits and visits[-1][0] == app and s - visits[-1][2] < 1:
            visits[-1][2], visits[-1][3] = e_, visits[-1][3] + act
        else:
            visits.append([app, s, e_, act])

    active = sum(s[2] for s in segs)
    idle_total = sum(subtract(max(s, since), min(e, until), []) for s, e in idle if e > since and s < until)
    switches = max(len(visits) - 1, 0)
    print(f"  Активное время: {fmt(active)}  (idle/lock/sleep вычтено: {fmt(idle_total)}).")
    if active:
        print(f"  Переключений между приложениями: {switches}  "
              f"(≈{switches / (active / 3600):.0f}/час, медиана визита {fmt(statistics.median([v[3] for v in visits if v[3] > 0] or [0]))}).")
    print(f"  Смен воркспейса: {ws_switch}.  Overview: {overview}.  Смен раскладки: {layout_sw}.")

    print("\n  По дням:")
    per_day = defaultdict(lambda: [0.0, 0, Counter()])
    for app, s, e_, act in visits:
        d = per_day[time.strftime("%Y-%m-%d %a %d.%m", time.localtime(s))]
        d[0] += act
        d[1] += 1
        d[2][app] += act
    table([[day[11:], fmt(v[0]), v[1], ", ".join(f"{a} {fmt(x)}" for a, x in v[2].most_common(3))]
           for day, v in sorted(per_day.items())],
          ["день", "активно", "визитов", "топ приложений"], ["<", ">", ">", "<"])

    print("\n  Приложения:")
    app_t, app_n, app_short = Counter(), Counter(), Counter()
    for app, s, e_, act in visits:
        app_t[app] += act
        app_n[app] += 1
        if 0 < act < 3:
            app_short[app] += 1
    table([[clip(a, 30), fmt(x), f"{100 * x / active:.0f}%" if active else "", app_n[a], app_short[a] or ""]
           for a, x in app_t.most_common(top)],
          ["app_id", "время", "доля", "визитов", "<3s"])

    print("\n  Окна/вкладки (заголовки):")
    tt = Counter()
    for s, e_, act, app, title in segs:
        tt[(app, title)] += act
    table([[clip(a, 16), title, fmt(x)] for (a, title), x in tt.most_common(top)], ["app", "заголовок", "время"],
          ["<", "<", ">"])

    # Пинг-понг: A → B (коротко) → A. Типично «подсмотреть/скопировать что-то и вернуться».
    pp, pp_t = Counter(), Counter()
    for a, b, c in zip(visits, visits[1:], visits[2:]):
        if a[0] == c[0] != b[0] and b[3] < 60:
            key = f"{a[0]} ⇄ {b[0]}"
            pp[key] += 1
            pp_t[key] += b[3]
    print("\n  Пинг-понг (ушёл в B меньше чем на минуту и вернулся) — сюда просится автоматизация:")
    table([[clip(k, 50), n, fmt(pp_t[k]), fmt(pp_t[k] / n)] for k, n in pp.most_common(top)],
          ["пара", "раз", "в B всего", "в среднем"])

    transit = [v for v in visits if 0 < v[3] < 1.5]
    print(f"\n  Транзитные фокусы (<1.5s, пролистывание окон по пути к нужному): {len(transit)}, "
          f"суммарно {fmt(sum(v[3] for v in transit))}.")
    tr = Counter(v[0] for v in transit)
    if tr:
        print("  Чаще всего пролетаешь через: " + ", ".join(f"{a} ({n})" for a, n in tr.most_common(5)))


def main():
    ap = argparse.ArgumentParser(description="Статистика активности: shell (atuin) + niri")
    ap.add_argument("-d", "--days", type=float, default=7, help="за сколько дней (по умолчанию 7)")
    ap.add_argument("-s", "--section", choices=["shell", "niri", "all"], default="all")
    ap.add_argument("-n", "--top", type=int, default=15, help="длина топов")
    args = ap.parse_args()
    until = time.time()
    since = until - args.days * 86400
    print(f"Период: {time.strftime('%d.%m %H:%M', time.localtime(since))} — "
          f"{time.strftime('%d.%m %H:%M', time.localtime(until))}")
    if args.section in ("shell", "all"):
        report_shell(load_shell(since), args.top)
    if args.section in ("niri", "all"):
        report_niri(since, until, args.top)
    print()


if __name__ == "__main__":
    main()
