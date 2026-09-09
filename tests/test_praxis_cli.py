from __future__ import annotations

import re

from praxis_cli.main import main


def test_main_prints_version_looking_string(capsys):
    main()

    captured = capsys.readouterr()
    assert re.match(r"\d+\.\d+\.\d+", captured.out.strip())
