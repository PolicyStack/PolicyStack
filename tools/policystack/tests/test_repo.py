"""copier's temp-dir cleanup race (Errno 39) is absorbed only for copier's own temp directories."""

import errno
import sys
import tempfile
from pathlib import Path

import pytest

from policystack import repo


def raise_in(path, code=errno.ENOTEMPTY):
    with repo._copier():
        raise OSError(code, "Directory not empty", str(path))


def test_copier_temp_dir_race_is_absorbed():
    top = Path(tempfile.mkdtemp(prefix="copier._main.new_copy."))
    (top / ".git" / "objects" / "pack").mkdir(parents=True)
    raise_in(top / ".git" / "objects" / "pack")
    assert not top.exists()


@pytest.mark.parametrize(
    "path,code",
    [
        ("/tmp/other/x", errno.ENOTEMPTY),
        ("/home/x", errno.ENOTEMPTY),
        (tempfile.gettempdir(), errno.ENOTEMPTY),
        (Path(tempfile.gettempdir(), "copier.x"), errno.EACCES),
    ],
)
def test_other_errors_raise(path, code):
    with pytest.raises(OSError):
        raise_in(path, code)


def test_stderr_hidden(capsys):
    with repo._copier():
        print("Make sure Git >= 2.24", file=sys.stderr)
    assert capsys.readouterr().err == ""
