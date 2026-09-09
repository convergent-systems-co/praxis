from __future__ import annotations

import re

from praxis_cli.main import main


def test_main_prints_version_looking_string(capsys):
    main()

    captured = capsys.readouterr()
    assert re.match(r"\d+\.\d+\.\d+", captured.out.strip())


def test_main_with_no_arguments_prints_the_version_and_returns_zero(capsys):
    # What the installed `praxis` console script does when it is run bare.
    # The call above cannot stand in for it: `main()` with no argument reads
    # `sys.argv[1:]`, which under pytest is pytest's own arguments, so it only
    # ever exercises the "first argument is not `executors`" half of the guard.
    exit_code = main([])

    captured = capsys.readouterr()
    assert exit_code == 0
    assert re.match(r"\d+\.\d+\.\d+", captured.out.strip())
