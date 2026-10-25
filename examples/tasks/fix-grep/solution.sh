#!/bin/sh
# Golden solution for fix-grep: lowercase before the substitution and
# trim leading/trailing dashes afterwards.
cd /task
python - <<'EOF'
import pathlib

p = pathlib.Path("app.py")
src = p.read_text()
src = src.replace("s = s.strip()", "s = s.strip().lower()", 1)
src = src.replace("return s", 'return s.strip("-")', 1)
p.write_text(src)
EOF
