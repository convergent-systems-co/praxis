#!/bin/sh
export PYTHONPATH=src
exec python3 -m pytest "$@"
