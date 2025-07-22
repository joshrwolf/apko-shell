#!/usr/bin/env apko-shell
#!apko-shell -p python3

echo "Running Python script with apko-shell"
echo "===================================="
echo

# The Python script is in the same directory as this script
# Since workdir is set to the script's directory, we can use relative path
python3 hello.py arg1 arg2 arg3
