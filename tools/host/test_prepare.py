"""Pure/mocked host-setup safety tests; never provision or execute host tools.

Parent invocation: /usr/bin/python3 -I -m unittest discover -s tools/host -p 'test_*.py'
Synthetic inventory below is test input, not deployment inventory or defaults.
"""
import copy
import importlib.util
import io
import pathlib
import stat
import sys
import types
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("lab_prepare", pathlib.Path(__file__).with_name("prepare.py"))
prepare = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(prepare)


class PreparationSafetyTests(unittest.TestCase):
    def test_duplicate_json_keys_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "duplicate JSON key"):
            prepare.decode('{"runtimes": {}, "runtimes": {}}')

    def test_traversal_rejects_noncanonical_paths(self):
        for path in ("relative", "//etc/config", "/etc/../etc/config", "/etc/./config", "/etc/config/"):
            with self.subTest(path=path), mock.patch.object(prepare.os, "lstat") as lookup:
                with self.assertRaisesRegex(RuntimeError, "noncanonical"):
                    prepare.trusted(path)
                lookup.assert_not_called()

    def test_traversal_rejects_user_writable_ancestor(self):
        def lookup(path):
            return types.SimpleNamespace(st_mode=stat.S_IFDIR | (0o777 if path == "/home" else 0o755), st_uid=0)
        with mock.patch.object(prepare.os, "lstat", side_effect=lookup):
            with self.assertRaisesRegex(RuntimeError, "not root-owned/protected: /home"):
                prepare.trusted("/home/lab/config")

    def test_traversal_rejects_symlink_ancestor(self):
        def lookup(path):
            return types.SimpleNamespace(st_mode=(stat.S_IFLNK | 0o777) if path == "/opt/lab" else (stat.S_IFDIR | 0o755), st_uid=0)
        with mock.patch.object(prepare.os, "lstat", side_effect=lookup):
            with self.assertRaisesRegex(RuntimeError, "unexpected type/symlink"):
                prepare.trusted("/opt/lab/config")

    def test_existing_directory_is_not_adopted(self):
        info = types.SimpleNamespace(st_mode=stat.S_IFDIR | 0o777, st_gid=0)
        with mock.patch.object(prepare, "trusted", return_value=info), mock.patch.object(prepare.os, "mkdir", side_effect=FileExistsError), mock.patch.object(prepare.os, "chown") as chown, mock.patch.object(prepare.os, "chmod") as chmod:
            with self.assertRaisesRegex(RuntimeError, "permission/identity mismatch"):
                prepare.directory("/safe/lab")
            chown.assert_not_called()
            chmod.assert_not_called()

    def inventory(self):
        return {"ipv4_pool": "10.201.0.0/16", "ipv6_pool": "fd12:3456:789a::/64", "resolvers": ["8.8.8.8"], "host_public_addresses": ["8.8.4.4"], "internal_prefixes": ["10.0.0.0/8"], "metadata_prefixes": ["169.254.169.254/32"], "inventory_revision": "unit-test-only"}

    def decode_inventory(self, value):
        tools = {name: "/usr/sbin/" + name for name in ("iptables", "ip6tables", "iptables-restore", "ip6tables-restore", "conntrack")}
        with mock.patch.object(prepare, "json_file", return_value=copy.deepcopy(value)), mock.patch.dict(prepare.TOOLS, tools):
            return prepare.inventory("/root/synthetic-unit-test.json")

    def test_inventory_requires_every_explicit_field(self):
        value = self.inventory()
        for key in value:
            missing = dict(value)
            del missing[key]
            with self.subTest(key=key), self.assertRaisesRegex(RuntimeError, "exactly"):
                self.decode_inventory(missing)

    def test_inventory_does_not_invent_empty_lists(self):
        for key in ("resolvers", "host_public_addresses", "internal_prefixes", "metadata_prefixes"):
            value = self.inventory()
            value[key] = []
            with self.subTest(key=key), self.assertRaisesRegex(RuntimeError, "nonempty"):
                self.decode_inventory(value)

    def test_inventory_rejects_documentation_or_private_resolver(self):
        for address in ("192.0.2.1", "10.0.0.1", "2001:db8::1", "64:ff9b::808:808"):
            value = self.inventory()
            value["resolvers"] = [address]
            with self.subTest(address=address), self.assertRaisesRegex(RuntimeError, "public numeric IPs"):
                self.decode_inventory(value)

    def test_inventory_rejects_unmasked_pool(self):
        value = self.inventory()
        value["ipv4_pool"] = "10.201.0.1/16"
        with self.assertRaises(ValueError):
            self.decode_inventory(value)

    def test_broad_internal_prefix_may_contain_dedicated_pool(self):
        value, pools = self.decode_inventory(self.inventory())
        self.assertEqual(value["internal_prefixes"], ["10.0.0.0/8"])
        self.assertEqual(value["backend"], "iptables")
        self.assertEqual(str(pools[0]), "10.201.0.0/16")
        self.assertEqual(value["iptables"], "/usr/sbin/iptables")

    def test_existing_backing_mismatch_never_formats(self):
        info = types.SimpleNamespace(st_dev=3, st_ino=4)
        with mock.patch.object(prepare.os.path, "lexists", return_value=True), mock.patch.object(prepare, "trusted", return_value=info), mock.patch.object(prepare, "json_file", return_value={}), mock.patch.object(prepare, "text", return_value="unit-test-uuid"), mock.patch.object(prepare, "command") as command:
            with self.assertRaisesRegex(RuntimeError, "never reformat"):
                prepare.storage(types.SimpleNamespace(pw_uid=900, pw_gid=900))
            command.assert_not_called()

    def test_foreign_runtime_name_never_overwritten(self):
        original = b'{"runtimes":{"nvidia":{"path":"nvidia-container-runtime","args":[]}}}'
        before = {"Runtimes": {prepare.RUNTIME_NAME: {"path": "/unrelated/runsc", "runtimeArgs": []}}}
        with mock.patch.object(prepare, "read_file", return_value=original), mock.patch.object(prepare, "command") as command, mock.patch.object(prepare, "atomic") as atomic:
            with self.assertRaisesRegex(RuntimeError, "already registered with different settings"):
                prepare.register_runtime(before)
            command.assert_not_called()
            atomic.assert_not_called()

    def test_identical_runtime_does_not_reload_docker(self):
        entry = {"path": prepare.INSTALL + "/runsc", "runtimeArgs": prepare.RUNTIME_ARGS}
        nvidia = {"path": "nvidia-container-runtime", "args": []}
        daemon = {"runtimes": {prepare.RUNTIME_NAME: entry, "nvidia": nvidia}, "log-driver": "json-file"}
        info = {"Runtimes": daemon["runtimes"], "DefaultRuntime": "runc"}
        with mock.patch.object(prepare, "read_file", return_value=prepare.serialized(daemon)), mock.patch.object(prepare, "docker", return_value=prepare.serialized(info)), mock.patch.object(prepare, "command") as command, mock.patch.object(prepare, "atomic") as atomic:
            prepare.register_runtime(info)
            command.assert_not_called()
            atomic.assert_not_called()

    def test_apply_cannot_be_abbreviated(self):
        arguments = ["prepare.py", "--app", "--inventory", "/root/unit-test.json", "--max-message-bytes", "1024", "--max-connections", "1", "--operation-timeout-seconds", "1", "--reconcile-interval-seconds", "1", "--api-deadline-seconds", "1", "--termination-deadline-seconds", "1", "--deadline-seconds", "30"]
        with mock.patch.object(sys, "argv", arguments), mock.patch("sys.stderr", new_callable=io.StringIO), mock.patch.object(prepare, "inspect_tools") as tools:
            with self.assertRaises(SystemExit) as result:
                prepare.main()
            self.assertEqual(result.exception.code, 2)
            tools.assert_not_called()


if __name__ == "__main__":
    unittest.main()
