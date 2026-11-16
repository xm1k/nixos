"""Exercise charge-control failure paths without touching the real battery."""

import contextlib
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import battery


class ThresholdTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        for name, filename in (("THRESHOLDS", "hardware"), ("CONFIG", "tlp.d/99-battery.conf"),
                               ("MAIN_CONFIG", "tlp.conf"), ("LOCK", "lock")):
            replacement = patch.object(battery, name, root / filename)
            replacement.start()
            self.addCleanup(replacement.stop)
        battery.THRESHOLDS.write_text("0 0\n")

    def apply(self, start, stop, hardware=None):
        def tlp_command(argv, check):
            self.assertEqual(argv, ["/test/tlp", "setcharge", str(start), str(stop), "BAT0"])
            self.assertTrue(check)
            battery.THRESHOLDS.write_text(hardware or f"{start} {stop}\n")
        with patch.object(battery.subprocess, "run", side_effect=tlp_command), contextlib.redirect_stdout(io.StringIO()):
            battery.set_thresholds(start, stop, "/test/tlp")

    def test_persist_only_verified_hardware_limits(self):
        self.apply(60, 80)
        self.assertIn("START_CHARGE_THRESH_BAT0=60\nSTOP_CHARGE_THRESH_BAT0=80\n", battery.CONFIG.read_text())
        previous = battery.CONFIG.read_text()
        with self.assertRaisesRegex(RuntimeError, "контроллер"):
            self.apply(70, 90, "60 80\n")
        self.assertEqual(previous, battery.CONFIG.read_text())

    def test_disabled_limits_accept_huawei_zero_zero(self):
        self.apply(0, 100, "0 0\n")
        self.assertIn("STOP_CHARGE_THRESH_BAT0=100", battery.CONFIG.read_text())

    def test_tlp_failure_keeps_saved_configuration(self):
        self.apply(60, 80)
        previous = battery.CONFIG.read_text()
        with patch.object(battery.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "tlp")):
            with self.assertRaises(subprocess.CalledProcessError):
                battery.set_thresholds(70, 90, "/test/tlp")
        self.assertEqual(previous, battery.CONFIG.read_text())

    def test_invalid_limits_never_call_tlp(self):
        for start, stop in ((80, 60), (80, 80), (-1, 80), (60, 101)):
            with self.subTest(start=start, stop=stop), patch.object(battery.subprocess, "run") as run:
                with self.assertRaises(ValueError):
                    battery.set_thresholds(start, stop, "/test/tlp")
                run.assert_not_called()
        self.assertFalse(battery.CONFIG.exists())

    def test_main_config_override_is_rejected(self):
        battery.MAIN_CONFIG.write_text("STOP_CHARGE_THRESH_BAT0=95\n")
        with patch.object(battery.subprocess, "run") as run:
            with self.assertRaisesRegex(RuntimeError, "services.tlp.settings"):
                battery.set_thresholds(60, 80, "/test/tlp")
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
