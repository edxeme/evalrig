package topk

import (
	"reflect"
	"testing"
)

func TestTopK(t *testing.T) {
	cases := []struct {
		name string
		nums []int
		k    int
		want []int
	}{
		{"k in the middle", []int{5, 1, 9, 3}, 2, []int{9, 5}},
		{"k covers all", []int{5, 1, 9, 3}, 10, []int{9, 5, 3, 1}},
		{"zero k", []int{5, 1, 9}, 0, nil},
		{"negative k", []int{5, 1, 9}, -2, nil},
		{"duplicates", []int{4, 4, 2, 4}, 3, []int{4, 4, 4}},
		{"empty input", nil, 3, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TopK(tc.nums, tc.k)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("TopK(%v, %d) = %v, want %v", tc.nums, tc.k, got, tc.want)
			}
		})
	}
}
