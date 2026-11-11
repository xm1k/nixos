#!/usr/bin/env python3
"""outlook — минимальный клиент Exchange (EWS) для скриптов и Claude.

  outlook search  — найти письма в папке (по теме, отправителю, дате)
  outlook show    — показать письмо целиком (тело текстом или HTML)
  outlook draft   — создать черновик (HTML/текст) в «Черновиках»
  outlook send    — сразу отправить письмо (копия в «Отправленных»)
  outlook delete  — удалить письмо/черновик (в «Удалённые»)
  outlook resolve — найти человека в адресной книге (должность, телефоны, офис)

Подключение (env):
  OUTLOOK_EWS_URL    адрес EWS            (по умолчанию exchange.yadro.com)
  OUTLOOK_USER       логин NTLM           (иначе $WORK_USER)
  OUTLOOK_PASS       пароль               (иначе $WORK_PASS, иначе файл $OUTLOOK_PASS_FILE)
  OUTLOOK_FROM       адрес «Кому» по умолчанию (иначе <user>@yadro.com)
"""

import argparse
import datetime as dt
import getpass
import html
import json
import os
import re
import sys
import warnings
import xml.etree.ElementTree as ET

import requests
from requests_ntlm import HttpNtlmAuth

warnings.filterwarnings("ignore")  # корпоративный сертификат без проверки

EWS_URL = os.environ.get("OUTLOOK_EWS_URL", "https://exchange.yadro.com/EWS/Exchange.asmx")
NS = {
    "s": "http://schemas.xmlsoap.org/soap/envelope/",
    "t": "http://schemas.microsoft.com/exchange/services/2006/types",
    "m": "http://schemas.microsoft.com/exchange/services/2006/messages",
}
FOLDERS = {"inbox", "sentitems", "drafts", "deleteditems", "outbox", "junkemail"}


def die(msg, code=1):
    print(f"outlook: {msg}", file=sys.stderr)
    sys.exit(code)


def user():
    u = os.environ.get("OUTLOOK_USER") or os.environ.get("WORK_USER")
    if not u:
        die("не задан логин: OUTLOOK_USER или WORK_USER")
    return u


_pass = None


def password():
    global _pass
    if _pass:
        return _pass
    p = os.environ.get("OUTLOOK_PASS") or os.environ.get("WORK_PASS")
    f = os.environ.get("OUTLOOK_PASS_FILE")
    if not p and f and os.path.exists(f):
        with open(f, encoding="utf-8") as fh:
            p = fh.read().strip()
    if not p and sys.stdin.isatty():
        p = getpass.getpass(f"Пароль для {user()}: ")
    if not p:
        die("нет пароля: OUTLOOK_PASS / WORK_PASS / OUTLOOK_PASS_FILE")
    _pass = p
    return p


def x(s):
    """Экранирование для XML."""
    return html.escape(s or "", quote=True)


def call(body):
    soap = f"""<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="{NS['s']}" xmlns:t="{NS['t']}" xmlns:m="{NS['m']}">
  <soap:Header><t:RequestServerVersion Version="Exchange2013"/></soap:Header>
  <soap:Body>{body}</soap:Body>
</soap:Envelope>"""
    sess = requests.Session()
    sess.trust_env = False  # Exchange ходит напрямую через VPN, корпоративный прокси ломает TLS
    try:
        r = sess.post(
            EWS_URL,
            data=soap.encode("utf-8"),
            headers={"Content-Type": "text/xml; charset=utf-8"},
            auth=HttpNtlmAuth(user(), password()),
            verify=False,
            timeout=60,
        )
    except requests.RequestException as e:
        die(f"нет связи с {EWS_URL}: {e} (VPN?)")
    if r.status_code == 401:
        die("401: неверный логин/пароль")
    try:
        root = ET.fromstring(r.content)
    except ET.ParseError:
        die(f"HTTP {r.status_code}: {r.text[:300]}")
    fault = root.find(".//s:Fault", NS)
    if fault is not None:
        die("SOAP fault: " + " ".join(fault.itertext()).strip()[:400])
    for msg in root.iter():
        if msg.tag.endswith("ResponseMessage") and msg.get("ResponseClass") == "Error":
            text = msg.findtext("m:MessageText", default="", namespaces=NS)
            code = msg.findtext("m:ResponseCode", default="", namespaces=NS)
            die(f"{code}: {text}")
    return root


def mailboxes(el, tag):
    out = []
    for mb in el.findall(f"t:{tag}/t:Mailbox", NS):
        name = mb.findtext("t:Name", default="", namespaces=NS)
        addr = mb.findtext("t:EmailAddress", default="", namespaces=NS)
        out.append(f"{name} <{addr}>" if name and addr and name != addr else (addr or name))
    return out


def item_dict(m):
    sender = mailboxes(m, "From") or mailboxes(m, "Sender")
    iid = m.find("t:ItemId", NS)
    return {
        "id": iid.get("Id") if iid is not None else None,
        "subject": m.findtext("t:Subject", default="", namespaces=NS),
        "sent": m.findtext("t:DateTimeSent", default="", namespaces=NS),
        "received": m.findtext("t:DateTimeReceived", default="", namespaces=NS),
        "from": sender[0] if sender else "",
        "to": mailboxes(m, "ToRecipients") or m.findtext("t:DisplayTo", default="", namespaces=NS),
        "cc": mailboxes(m, "CcRecipients") or m.findtext("t:DisplayCc", default="", namespaces=NS),
    }


def folder_xml(name):
    if name not in FOLDERS:
        die(f"неизвестная папка {name!r}; доступны: {', '.join(sorted(FOLDERS))}")
    return f'<t:DistinguishedFolderId Id="{name}"/>'


# ---------------------------------------------------------------- search

def find(a, limit):
    conds = []
    if a.subject:
        conds.append(
            '<t:Contains ContainmentMode="Substring" ContainmentComparison="IgnoreCase">'
            '<t:FieldURI FieldURI="item:Subject"/>'
            f'<t:Constant Value="{x(a.subject)}"/></t:Contains>'
        )
    if a.sender:
        conds.append(
            '<t:Contains ContainmentMode="Substring" ContainmentComparison="IgnoreCase">'
            '<t:FieldURI FieldURI="message:From"/>'
            f'<t:Constant Value="{x(a.sender)}"/></t:Contains>'
        )
    if a.days:
        since = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=a.days)).strftime("%Y-%m-%dT%H:%M:%SZ")
        conds.append(
            '<t:IsGreaterThanOrEqualTo><t:FieldURI FieldURI="item:DateTimeReceived"/>'
            f'<t:FieldURIOrConstant><t:Constant Value="{since}"/></t:FieldURIOrConstant>'
            "</t:IsGreaterThanOrEqualTo>"
        )
    restriction = ""
    if conds:
        inner = conds[0] if len(conds) == 1 else "<t:And>" + "".join(conds) + "</t:And>"
        restriction = f"<m:Restriction>{inner}</m:Restriction>"

    body = f"""<m:FindItem Traversal="Shallow">
  <m:ItemShape><t:BaseShape>IdOnly</t:BaseShape><t:AdditionalProperties>
    <t:FieldURI FieldURI="item:Subject"/><t:FieldURI FieldURI="item:DateTimeSent"/>
    <t:FieldURI FieldURI="item:DateTimeReceived"/><t:FieldURI FieldURI="message:From"/>
    <t:FieldURI FieldURI="item:DisplayTo"/><t:FieldURI FieldURI="item:DisplayCc"/>
  </t:AdditionalProperties></m:ItemShape>
  <m:IndexedPageItemView MaxEntriesReturned="{limit}" Offset="0" BasePoint="Beginning"/>
  {restriction}
  <m:SortOrder><t:FieldOrder Order="Descending"><t:FieldURI FieldURI="item:DateTimeReceived"/></t:FieldOrder></m:SortOrder>
  <m:ParentFolderIds>{folder_xml(a.folder)}</m:ParentFolderIds>
</m:FindItem>"""
    root = call(body)
    return [item_dict(m) for m in root.iter(f"{{{NS['t']}}}Message")]


def cmd_search(a):
    items = find(a, a.limit)
    if a.json:
        json.dump(items, sys.stdout, ensure_ascii=False, indent=2)
        print()
        return
    if not items:
        print("ничего не найдено", file=sys.stderr)
        return
    for i in items:
        when = (i["sent"] or i["received"])[:16].replace("T", " ")
        who = i["to"] if a.folder in ("sentitems", "drafts") else i["from"]
        print(f"{when}  {i['subject']}\n    {who}\n    id: {i['id']}")


# ---------------------------------------------------------------- show

def get_items(ids, body_type):
    id_xml = "".join(f'<t:ItemId Id="{x(i)}"/>' for i in ids)
    body = f"""<m:GetItem>
  <m:ItemShape><t:BaseShape>Default</t:BaseShape><t:BodyType>{body_type}</t:BodyType>
  <t:AdditionalProperties><t:FieldURI FieldURI="item:Body"/><t:FieldURI FieldURI="message:CcRecipients"/>
  <t:FieldURI FieldURI="item:DateTimeSent"/></t:AdditionalProperties></m:ItemShape>
  <m:ItemIds>{id_xml}</m:ItemIds>
</m:GetItem>"""
    root = call(body)
    out = []
    for m in root.iter(f"{{{NS['t']}}}Message"):
        d = item_dict(m)
        d["body"] = m.findtext("t:Body", default="", namespaces=NS)
        out.append(d)
    return out


def cmd_show(a):
    if a.id:
        ids = [a.id]
    else:
        found = find(a, 1)
        if not found:
            die("подходящих писем не найдено")
        ids = [found[0]["id"]]
    items = get_items(ids, "HTML" if a.html else "Text")
    if a.json:
        json.dump(items[0], sys.stdout, ensure_ascii=False, indent=2)
        print()
        return
    for i in items:
        if not a.body_only:
            j = lambda v: ", ".join(v) if isinstance(v, list) else v
            print(f"Тема:   {i['subject']}")
            print(f"Дата:   {(i['sent'] or i['received'])[:16].replace('T', ' ')}")
            print(f"От:     {i['from']}")
            print(f"Кому:   {j(i['to'])}")
            if i["cc"]:
                print(f"Копия:  {j(i['cc'])}")
            print()
        print(i["body"].replace("\r\n", "\n").rstrip())


# ---------------------------------------------------------------- draft / send

def read_body(path):
    if path == "-":
        return sys.stdin.read()
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def recipients(tag, addrs):
    addrs = [s.strip() for a in addrs for s in a.split(",") if s.strip()]
    if not addrs:
        return ""
    mbs = "".join(f"<t:Mailbox><t:EmailAddress>{x(a)}</t:EmailAddress></t:Mailbox>" for a in addrs)
    return f"<t:{tag}>{mbs}</t:{tag}>"


def create(a, disposition):
    content = read_body(a.body)
    if a.text:
        is_html = False
    else:
        is_html = a.html or a.body.endswith((".html", ".htm")) or bool(re.search(r"<(html|body|p|div|br|ul|table)\b", content, re.I))
    to = a.to or [os.environ.get("OUTLOOK_FROM") or f"{user()}@yadro.com"]
    folder = "drafts" if disposition == "SaveOnly" else "sentitems"
    body = f"""<m:CreateItem MessageDisposition="{disposition}">
  <m:SavedItemFolderId>{folder_xml(folder)}</m:SavedItemFolderId>
  <m:Items><t:Message>
    <t:Subject>{x(a.subject)}</t:Subject>
    <t:Body BodyType="{'HTML' if is_html else 'Text'}">{x(content)}</t:Body>
    {recipients('ToRecipients', to)}{recipients('CcRecipients', a.cc or [])}
  </t:Message></m:Items>
</m:CreateItem>"""
    root = call(body)
    iid = root.find(".//t:ItemId", NS)
    return (iid.get("Id"), iid.get("ChangeKey")) if iid is not None else (None, None)


def inline_attachments(item_id):
    """Inline-вложения письма (картинки подписи и т.п.): [{name, type, cid, content(b64)}]."""
    root = call(f"""<m:GetItem>
  <m:ItemShape><t:BaseShape>IdOnly</t:BaseShape><t:AdditionalProperties>
    <t:FieldURI FieldURI="item:Attachments"/></t:AdditionalProperties></m:ItemShape>
  <m:ItemIds><t:ItemId Id="{x(item_id)}"/></m:ItemIds>
</m:GetItem>""")
    ids = [
        fa.find("t:AttachmentId", NS).get("Id")
        for fa in root.iter(f"{{{NS['t']}}}FileAttachment")
        if fa.findtext("t:ContentId", default="", namespaces=NS)
    ]
    if not ids:
        return []
    id_xml = "".join(f'<t:AttachmentId Id="{x(i)}"/>' for i in ids)
    root = call(f"<m:GetAttachment><m:AttachmentIds>{id_xml}</m:AttachmentIds></m:GetAttachment>")
    return [
        {
            "name": fa.findtext("t:Name", default="", namespaces=NS),
            "type": fa.findtext("t:ContentType", default="application/octet-stream", namespaces=NS),
            "cid": fa.findtext("t:ContentId", default="", namespaces=NS),
            "content": fa.findtext("t:Content", default="", namespaces=NS),
        }
        for fa in root.iter(f"{{{NS['t']}}}FileAttachment")
    ]


def attach(item_id, change_key, atts):
    files = "".join(
        f"""<t:FileAttachment><t:Name>{x(a['name'])}</t:Name><t:ContentType>{x(a['type'])}</t:ContentType>
<t:ContentId>{x(a['cid'])}</t:ContentId><t:IsInline>true</t:IsInline><t:Content>{a['content']}</t:Content></t:FileAttachment>"""
        for a in atts
    )
    call(f"""<m:CreateAttachment>
  <m:ParentItemId Id="{x(item_id)}" ChangeKey="{x(change_key)}"/>
  <m:Attachments>{files}</m:Attachments>
</m:CreateAttachment>""")


def cmd_draft(a):
    atts = inline_attachments(a.inline_from) if a.inline_from else []
    iid, ck = create(a, "SaveOnly")
    if atts and iid:
        attach(iid, ck, atts)
    extra = f", inline-вложений: {len(atts)}" if atts else ""
    print(f"Черновик создан: «{a.subject}»{extra}", file=sys.stderr)
    if iid:
        print(iid)


def cmd_send(a):
    if a.inline_from:
        die("--inline-from поддерживается только в draft")
    create(a, "SendAndSaveCopy")
    print(f"Отправлено: «{a.subject}»", file=sys.stderr)


def cmd_delete(a):
    ids = "".join(f'<t:ItemId Id="{x(i)}"/>' for i in a.ids)
    call(f'''<m:DeleteItem DeleteType="MoveToDeletedItems"><m:ItemIds>{ids}</m:ItemIds></m:DeleteItem>''')
    print(f"Удалено: {len(a.ids)} (в «Удалённых»)", file=sys.stderr)


def cmd_resolve(a):
    root = call(f"""<m:ResolveNames ReturnFullContactData="true" SearchScope="ActiveDirectory" ContactDataShape="AllProperties">
  <m:UnresolvedEntry>{x(a.query or user())}</m:UnresolvedEntry>
</m:ResolveNames>""")
    out = []
    for r in root.iter(f"{{{NS['t']}}}Resolution"):
        mb, c = r.find("t:Mailbox", NS), r.find("t:Contact", NS)
        d = {
            "name": mb.findtext("t:Name", default="", namespaces=NS),
            "email": mb.findtext("t:EmailAddress", default="", namespaces=NS),
        }
        if c is not None:
            for k, tag in (("title", "JobTitle"), ("department", "Department"), ("company", "CompanyName"),
                           ("office", "OfficeLocation"), ("given", "GivenName"), ("surname", "Surname")):
                v = c.findtext(f"t:{tag}", default="", namespaces=NS)
                if v:
                    d[k] = v
            phones = {e.get("Key"): e.text for e in c.findall("t:PhoneNumbers/t:Entry", NS) if e.text}
            if phones:
                d["phones"] = phones
            for e in c.findall("t:PhysicalAddresses/t:Entry", NS):
                addr = {t.tag.split("}")[1].lower(): t.text for t in e if t.text}
                if addr:
                    d.setdefault("addresses", {})[e.get("Key")] = addr
        out.append(d)
    if not out:
        die("никого не найдено")
    json.dump(out if len(out) > 1 else out[0], sys.stdout, ensure_ascii=False, indent=2)
    print()


# ---------------------------------------------------------------- cli

def main():
    p = argparse.ArgumentParser(prog="outlook", description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)

    def filters(sp, default_folder):
        sp.add_argument("-f", "--folder", default=default_folder,
                        help=f"папка (по умолчанию {default_folder}): " + ", ".join(sorted(FOLDERS)))
        sp.add_argument("-s", "--subject", help="подстрока в теме")
        sp.add_argument("--from", dest="sender", help="подстрока в отправителе")
        sp.add_argument("-d", "--days", type=int, help="только за последние N дней")

    s = sub.add_parser("search", aliases=["ls"], help="найти письма (новые сверху)")
    filters(s, "inbox")
    s.add_argument("-n", "--limit", type=int, default=20)
    s.add_argument("--json", action="store_true")
    s.set_defaults(fn=cmd_search)

    s = sub.add_parser("show", aliases=["cat"], help="показать письмо: по id или самое свежее по фильтрам")
    s.add_argument("id", nargs="?", help="id из search; без него — последнее письмо по фильтрам")
    filters(s, "sentitems")
    s.add_argument("--html", action="store_true", help="тело в HTML (по умолчанию — текст)")
    s.add_argument("-b", "--body-only", action="store_true", help="только тело")
    s.add_argument("--json", action="store_true")
    s.set_defaults(fn=cmd_show)

    for name, fn, hlp in (("draft", cmd_draft, "создать черновик"), ("send", cmd_send, "отправить письмо")):
        s = sub.add_parser(name, help=hlp)
        s.add_argument("-s", "--subject", required=True)
        s.add_argument("-t", "--to", action="append",
                       help="получатель (повтором или через запятую); по умолчанию — себе")
        s.add_argument("-c", "--cc", action="append")
        s.add_argument("-b", "--body", default="-", help="файл с телом или '-' (stdin)")
        s.add_argument("--inline-from", metavar="ID",
                       help="скопировать inline-картинки (cid:, например из подписи) из письма ID")
        g = s.add_mutually_exclusive_group()
        g.add_argument("--html", action="store_true", help="тело — HTML (по умолчанию угадывается)")
        g.add_argument("--text", action="store_true", help="тело — простой текст")
        s.set_defaults(fn=fn)

    s = sub.add_parser("delete", aliases=["rm"], help="удалить письма по id (в «Удалённые»)")
    s.add_argument("ids", nargs="+")
    s.set_defaults(fn=cmd_delete)

    s = sub.add_parser("resolve", aliases=["who"], help="человек из адресной книги (по умолчанию — я), JSON")
    s.add_argument("query", nargs="?", help="логин, имя или адрес")
    s.set_defaults(fn=cmd_resolve)

    a = p.parse_args()
    a.fn(a)


if __name__ == "__main__":
    main()
