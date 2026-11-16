"""PolicyStack launcher. Imported before any submodule, so before copier."""

import os

# copier runs git through plumbum, which copies os.environ once at import, so this must come first.
# Every git commit otherwise spawns a detached `git maintenance run --auto`, which can write into
# copier's temp repository while copier removes it (Errno 39). Appended: the last entry wins.
_n = int(os.environ.get("GIT_CONFIG_COUNT", "0"))
for _i, (_k, _v) in enumerate((("gc.auto", "0"), ("maintenance.auto", "false")), _n):
    os.environ[f"GIT_CONFIG_KEY_{_i}"], os.environ[f"GIT_CONFIG_VALUE_{_i}"] = _k, _v
os.environ["GIT_CONFIG_COUNT"] = str(_n + 2)
