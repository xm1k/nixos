"""Huawei MateBook charge limits, persisted through TLP."""

import argparse
import fcntl
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

BATTERY = Path("/sys/class/power_supply/BAT0")
THRESHOLDS = Path("/sys/devices/platform/huawei-wmi/charge_control_thresholds")
CONFIG = Path("/etc/tlp.d/99-battery.conf")
MAIN_CONFIG = Path("/etc/tlp.conf")
LOCK = Path("/run/lock/battery.lock")
PRESETS = {"desk": (60, 80), "daily": (70, 90), "travel": (95, 100), "off": (0, 100)}

HELP = """battery — состояние батареи и постоянные пороги зарядки Huawei MateBook

Команды:
  battery [status]            Заряд, состояние, ёмкость, циклы и пороги
  battery threshold          Текущие аппаратные и сохранённые пороги
  battery threshold STOP     Установить START=max(0, STOP-20), STOP
  battery set START STOP     Установить оба порога (0 <= START < STOP <= 100)
  battery desk               60–80%: преимущественно работа от розетки
  battery daily              70–90%: компромисс для смешанного использования
  battery travel             95–100%: полный заряд перед поездкой
  battery off                0–100%: отключить пользовательское ограничение
  battery help               Эта справка

Изменение порогов требует sudo; команда запросит его сама. Пороги применяются
сразу и сохраняются в /etc/tlp.d/99-battery.conf. TLP восстанавливает их при
загрузке; файл сохраняется между nixos-rebuild. Все режимы постоянные: после
поездки верни battery desk или battery daily самостоятельно.

Как работают пороги:
  START — уровень, ниже которого разрешается начать зарядку от сети.
  STOP  — уровень, на котором зарядка прекращается.
  Например, set 60 80 не требует циклически разряжать батарею до 60%.
  Если сейчас 100%, установка STOP=80 не разрядит батарею принудительно.
  Проценты заряда считаются от нынешней полной ёмкости, а не заводской.
  Huawei может показывать аппаратные 0 0 для отключённых ограничений.

Как сохранить ресурс ёмкости:
  • За столом используй desk (60–80%) и питание от сети. Не нужно специально
    отключать зарядку и расходовать циклы ради соблюдения диапазона.
  • Перед долгой работой без розетки используй travel. Полный заряд допустим,
    но не стоит неделями держать горячую батарею на 100%.
  • Избегай перегрева: открытые вентиляционные отверстия, твёрдая поверхность,
    без хранения заряженного ноутбука в горячей машине.
  • Не разряжай регулярно до 0%. Подзаряжай по необходимости; частичные
    подзарядки допустимы. Регулярная «калибровка» не восстанавливает ёмкость.
  • Для хранения на несколько месяцев оставь около 40–60%, выключи ноутбук
    и периодически проверяй заряд, чтобы не допустить глубокого разряда.
  • Лимит 80% уменьшает доступную автономность сейчас ради замедления износа.
    Уже утраченную химическую ёмкость пороги и программные настройки не вернут.

Как дольше работать от одного заряда:
  Уменьши яркость до комфортной, отключи ненужную периферию и внешний экран,
  проверь фоновые процессы и аппаратное декодирование видео. Экономичный
  профиль CPU помогает при нагрузке. Сравни 48 и 60 Гц на встроенном экране.
  Порог зарядки сам по себе не снижает потребление работающего ноутбука.

Показатель «остаточная ёмкость» — оценка контроллера full/design, а не
независимое измерение. Мгновенная мощность батареи информативна при разряде
без подключённого питания, а при зарядке не равна расходу всего ноутбука.

Источники:
  https://linrunner.de/tlp/settings/battery.html
  https://consumer.huawei.com/ca/support/content/en-us00792538/
  https://www.apple.com/batteries/maximizing-performance/
"""


def read(path):
    try:
        return path.read_text().strip()
    except OSError:
        return None


def number(name):
    value = read(BATTERY / name)
    return int(value) if value is not None else None


def show_thresholds():
    current = read(THRESHOLDS)
    if current is None:
        raise RuntimeError("интерфейс порогов huawei-wmi недоступен")
    print(f"Аппаратные пороги START STOP: {current}%" +
          (" (ограничение отключено)" if current in ("0 0", "0 100") else ""))
    saved = read(CONFIG)
    if saved is None:
        print("Сохранённые пороги battery: не заданы")
    else:
        values = re.findall(r"^(?:START|STOP)_CHARGE_THRESH_BAT0=(\d+)$", saved, re.M)
        print("Сохранённые пороги battery: " + " ".join(values) + "%")


def status():
    if not BATTERY.exists():
        raise RuntimeError("батарея BAT0 не найдена")
    print(f"Батарея: {read(BATTERY / 'model_name') or 'BAT0'}")
    print(f"Заряд: {read(BATTERY / 'capacity') or '?'}%; состояние: {read(BATTERY / 'status') or '?'}")
    full, design = number("energy_full"), number("energy_full_design")
    if full is not None and design:
        print(f"Полная ёмкость: {full / 1e6:.2f} / {design / 1e6:.2f} Вт·ч; "
              f"остаточная ёмкость: {100 * full / design:.1f}% (оценка контроллера)")
    print(f"Циклы: {read(BATTERY / 'cycle_count') or '?'}")
    power = number("power_now")
    if power is not None:
        print(f"Мощность на батарее: {power / 1e6:.2f} Вт (не общий расход при питании от сети)")
    show_thresholds()


def validate(start, stop):
    if not 0 <= start < stop <= 100:
        raise ValueError("нужно 0 <= START < STOP <= 100")


def set_thresholds(start, stop, tlp):
    validate(start, stop)
    if not THRESHOLDS.exists():
        raise RuntimeError("интерфейс порогов huawei-wmi недоступен")
    # tlp.conf is loaded after drop-ins: reject overrides rather than silently
    # saving limits that TLP would undo on the next boot.
    main = read(MAIN_CONFIG) or ""
    if re.search(r"^\s*(?:START|STOP)_CHARGE_THRESH_BAT0\s*=", main, re.M):
        raise RuntimeError("пороги уже заданы в /etc/tlp.conf; убери их из services.tlp.settings")
    CONFIG.parent.mkdir(parents=True, exist_ok=True)
    with LOCK.open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        subprocess.run([tlp, "setcharge", str(start), str(stop), "BAT0"], check=True)
        actual = read(THRESHOLDS)
        expected = f"{start} {stop}"
        if actual != expected and not ((start, stop) == (0, 100) and actual == "0 0"):
            raise RuntimeError(f"контроллер вернул пороги {actual!r}, ожидались {expected}; конфигурация не сохранена")
        content = ("# Managed by battery; do not edit.\n"
                   f"START_CHARGE_THRESH_BAT0={start}\nSTOP_CHARGE_THRESH_BAT0={stop}\n")
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(mode="w", dir=CONFIG.parent, delete=False) as output:
                temporary = Path(output.name)
                output.write(content)
                output.flush()
                os.fsync(output.fileno())
                os.fchmod(output.fileno(), 0o644)
            temporary.replace(CONFIG)
        except OSError as error:
            raise RuntimeError(f"пороги применены, но сохранить их не удалось: {error}") from error
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)
    print(f"Установлено и сохранено: START={start}%, STOP={stop}%.")


def main():
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("-h", "--help", action="store_true")
    parser.add_argument("--tlp", default="/run/current-system/sw/bin/tlp")
    parser.add_argument("command", nargs="?", default="status")
    parser.add_argument("values", nargs="*", type=int)
    args = parser.parse_args()
    command, values = args.command, args.values
    if args.help or command == "help":
        print(HELP)
        return
    if command in ("status", "threshold") and not values:
        (status if command == "status" else show_thresholds)()
        return
    if command in PRESETS and not values:
        start, stop = PRESETS[command]
    elif command == "set" and len(values) == 2:
        start, stop = values
    elif command == "threshold" and len(values) == 1:
        stop = values[0]
        start = max(0, stop - 20)
    elif command == "threshold" and len(values) == 2:
        start, stop = values
    else:
        raise ValueError("неверная команда или аргументы; см. battery help")
    validate(start, stop)
    if os.geteuid() != 0:
        os.execv("/run/wrappers/bin/sudo", ["sudo", "--", sys.executable,
                 str(Path(__file__).resolve()), "--tlp", args.tlp, "set", str(start), str(stop)])
    set_thresholds(start, stop, args.tlp)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"battery: {error}", file=sys.stderr)
        sys.exit(1)
