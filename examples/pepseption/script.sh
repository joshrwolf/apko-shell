#!/usr/bin/env apko-shell
#!apko-shell -p python3,uv
# /// apko
# paths:
#   - path: /.cache
#     type: directory
#     uid: 65532
#     gid: 65532
#     permissions: 0o777
# ///

echo "Running Python script with PEP 723 dependencies"
echo "=============================================="
echo

# Use uv to run the script with its inline dependencies
uv run hello.py
