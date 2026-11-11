#!/usr/bin/env python3
"""Текстовый недельный отчёт (дерево на отступах) → HTML-письмо в стиле прошлых отчётов.

  render.py report.txt [--signature-from prev.html] > report.html

report.txt — пункты, вложенность отступом (4 пробела или таб на уровень):

    Автоматизация
        Пайплайн пересборки stability:
            Восстановлено логгирование ...
    Поддержка
        ...

Подпись («С уважением, …») в report.txt не пишется: по умолчанию берётся корпоративная
подпись YADRO из ../signature.html (логотип по внешней ссылке, вложения не нужны).
--signature-from prev.html — взять подпись из HTML другого письма.
"""

import argparse
import html
import os
import re
import sys

SIGNATURE_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "signature.html")

FONT = 'face="Calibri,sans-serif" style="font-family:Calibri,sans-serif"'
FALLBACK_SIGNATURE = (
    f'<p><font {FONT}><span style="font-size:11pt">С уважением,<br>&nbsp;&nbsp;<b>Смирнов Д.А.</b></span></font></p>'
)


def parse(text):
    """→ [(level, text)], без пустых строк и строк подписи."""
    rows = []
    for raw in text.splitlines():
        line = raw.replace("\t", "    ").rstrip()
        if not line.strip() or line.strip() == "```":
            continue
        if re.match(r"\s*(С уважением|Смирнов Д)", line):
            break
        indent = len(line) - len(line.lstrip(" "))
        rows.append((indent, re.sub(r"^\s*[-*•]\s+", "", line).strip()))
    if not rows:
        sys.exit("render: пустой отчёт")
    # нормализуем отступы в уровни 0,1,2… по возрастанию уникальных значений
    levels = sorted({i for i, _ in rows})
    return [(levels.index(i), t) for i, t in rows]


def build(rows, i=0, level=0):
    """Рекурсивно строит <ul>; возвращает (html, следующий индекс)."""
    out = ['<ul style="margin-top:0;margin-bottom:0">']
    while i < len(rows) and rows[i][0] >= level:
        lvl, text = rows[i]
        if lvl > level:  # пропуск уровня — вложим
            sub, i = build(rows, i, lvl)
            out.append(f"<li style=\"list-style:none\">{sub}</li>")
            continue
        has_children = i + 1 < len(rows) and rows[i + 1][0] > level
        t = html.escape(text)
        if level == 1 and has_children and len(text) <= 60:  # подзаголовок раздела
            t = f"<b>{t}</b>"
        li = f'<li style="margin:0"><font {FONT}><span style="font-size:11pt">{t}</span></font>'
        i += 1
        if has_children:
            sub, i = build(rows, i, level + 1)
            li += sub
        out.append(li + "</li>")
    out.append("</ul>")
    return "".join(out), i


def top_level(rows):
    """Разделы верхнего уровня — отдельными <ul> с отступом сверху, как в прошлых письмах."""
    parts, i = [], 0
    while i < len(rows):
        j = i + 1
        while j < len(rows) and rows[j][0] > 0:
            j += 1
        block, _ = build(rows[i:j])
        parts.append(block.replace('margin-top:0', 'margin-top:14pt', 1))
        i = j
    return "\n".join(parts)


def signature(path):
    if not path:
        if os.path.exists(SIGNATURE_FILE):
            return open(SIGNATURE_FILE, encoding="utf-8").read()
        return FALLBACK_SIGNATURE
    src = open(path, encoding="utf-8").read()
    pos = src.find("уважением")
    if pos < 0:
        return FALLBACK_SIGNATURE
    start = src.rfind("<table", 0, pos)
    end = src.find("</table>", pos)
    if start < 0 or end < 0:
        return FALLBACK_SIGNATURE
    return src[start:end + len("</table>")]


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("report", help="текстовый отчёт или '-'")
    p.add_argument("--signature-from", help="взять подпись из HTML письма (outlook show --html -b) вместо signature.html")
    a = p.parse_args()
    text = sys.stdin.read() if a.report == "-" else open(a.report, encoding="utf-8").read()
    body = top_level(parse(text))
    print(f"""<html><head><meta http-equiv="Content-Type" content="text/html; charset=utf-8"></head>
<body dir="ltr"><div style="font-size:11pt;color:#000000;font-family:Calibri,Helvetica,sans-serif;">
{body}
<br><br>
{signature(a.signature_from)}
<br></div></body></html>""")


if __name__ == "__main__":
    main()
