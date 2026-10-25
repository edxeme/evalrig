import csv


def main():
    with open("data.csv") as f:
        rows = list(csv.DictReader(f))
    # TODO: print "region: total" lines, highest total first,
    # ties broken alphabetically.
    print("TODO")


if __name__ == "__main__":
    main()
