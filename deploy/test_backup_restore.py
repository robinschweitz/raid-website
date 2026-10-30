"""Exercise the actual embedded restore launcher without production credentials."""
import base64
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


source = (Path(__file__).parent / "bin" / "raid-website-backup").read_text(encoding="utf-8")
launcher = source.split("<<'RESTORE_PY'\n", 1)[1].split("\nRESTORE_PY", 1)[0]
namespace = {"__name__": "restore_test"}
exec(compile(launcher, "restore-launcher", "exec"), namespace)
restore_environment = namespace["restore_environment"]


class RestoreEnvironmentTest(unittest.TestCase):
    def load(self, text):
        with tempfile.TemporaryDirectory() as directory:
            env_file = Path(directory) / "archive.env"
            env_file.write_text(text, encoding="utf-8")
            return restore_environment(env_file, "/isolated/restore")

    def test_archived_current_and_previous_keys_are_used(self):
        key = base64.b64encode(b"a" * 32).decode()
        previous = base64.b64encode(b"b" * 32).decode()
        with patch.dict("os.environ", {"TOTP_ENCRYPTION_KEY": "wrong-live-key"}):
            result = self.load(f'TOTP_ENCRYPTION_KEY="{key}"\r\nTOTP_ENCRYPTION_KEYS_PREVIOUS=\'{previous}, {key}\'\n')
        self.assertEqual(result["TOTP_ENCRYPTION_KEY"], key)
        self.assertEqual(result["TOTP_ENCRYPTION_KEYS_PREVIOUS"], previous + ", " + key)
        self.assertEqual(result["DATABASE_URL"], "file:/isolated/restore/raid.db?_pragma=journal_mode(WAL)")

    def test_production_configuration_and_shell_text_are_not_inherited(self):
        text = 'ENV=production\nLISTEN_ADDR=0.0.0.0:8080\nDATABASE_URL=file:/live.db\nVAPID_PRIVATE_KEY=secret\nADMIN_PASSWORD=secret\n$(exit 1)\n'
        with patch.dict("os.environ", {"RISING_GODS_PROXY_TOKEN": "live-secret"}):
            result = self.load(text)
        self.assertEqual(set(result), {"PATH", "ENV", "LISTEN_ADDR", "DATABASE_URL"})
        self.assertEqual(result["ENV"], "development")
        self.assertEqual(result["LISTEN_ADDR"], "127.0.0.1:18080")

    def test_invalid_keys_fail_without_disclosing_values(self):
        key = base64.b64encode(b"a" * 32).decode()
        for name, value in [("TOTP_ENCRYPTION_KEY", "private-bad-key"),
                            ("TOTP_ENCRYPTION_KEY", "'unclosed-secret"),
                            ("TOTP_ENCRYPTION_KEY", "$(echo secret)"),
                            ("TOTP_ENCRYPTION_KEY", base64.b64encode(b"short").decode()),
                            ("TOTP_ENCRYPTION_KEYS_PREVIOUS", key + ",")]:
            with self.subTest(name=name, value=value):
                with self.assertRaises(SystemExit) as failure:
                    self.load(name + "=" + value)
                self.assertEqual(str(failure.exception), "invalid archived " + name)

    def test_missing_key_does_not_fall_back_to_the_live_environment(self):
        with patch.dict("os.environ", {"TOTP_ENCRYPTION_KEY": "live-secret"}):
            self.assertNotIn("TOTP_ENCRYPTION_KEY", self.load(""))
        result = self.load("TOTP_ENCRYPTION_KEY=\nTOTP_ENCRYPTION_KEYS_PREVIOUS=\"\"\n")
        self.assertEqual(result["TOTP_ENCRYPTION_KEY"], "")
        self.assertEqual(result["TOTP_ENCRYPTION_KEYS_PREVIOUS"], "")
