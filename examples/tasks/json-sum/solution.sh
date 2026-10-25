#!/bin/sh
# Golden solution for json-sum: implement TopK.
cd /task
cat > main.go <<'EOF'
package topk

import "sort"

func TopK(nums []int, k int) []int {
	if k <= 0 {
		return nil
	}
	sorted := append([]int(nil), nums...)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
	if k > len(sorted) {
		k = len(sorted)
	}
	return sorted[:k]
}
EOF
