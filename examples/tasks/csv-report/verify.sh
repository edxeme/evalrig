#!/bin/sh
cd /task
python main.py > /tmp/out.txt
diff -u /task/.evalrig/expected.txt /tmp/out.txt
