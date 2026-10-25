#!/bin/sh
# Golden solution for csv-report.
cd /task
cat > main.py <<'EOF'
import csv
from collections import defaultdict


def main():
    totals = defaultdict(int)
    with open("data.csv") as f:
        for row in csv.DictReader(f):
            totals[row["region"]] += int(row["revenue"])
    for region, total in sorted(totals.items(), key=lambda kv: (-kv[1], kv[0])):
        print(f"{region}: {total}")


if __name__ == "__main__":
    main()
EOF
