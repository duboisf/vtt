package recall

import "testing"

func TestParseSelection(t *testing.T) {
	available := []int64{2, 3, 4, 5, 6, 7, 10}

	cases := []struct {
		name    string
		input   string
		want    []int64
		wantErr bool
	}{
		{"single", "3", []int64{3}, false},
		{"closed range", "3-5", []int64{3, 4, 5}, false},
		{"open end", "5-", []int64{5, 6, 7, 10}, false},
		{"open start", "-4", []int64{2, 3, 4}, false},
		{"comma mix", "2,5-7", []int64{2, 5, 6, 7}, false},
		{"overlap dedupes", "3-5,4-6", []int64{3, 4, 5, 6}, false},
		{"whitespace", " 3 , 5-6 ", []int64{3, 5, 6}, false},
		{"all keyword", "all", available, false},
		{"star", "*", available, false},
		{"range with gaps in buffer", "4-11", []int64{4, 5, 6, 7, 10}, false},
		{"empty", "", nil, true},
		{"bare unknown id errors", "99", nil, true},
		{"non numeric", "abc", nil, true},
		{"reversed range", "7-3", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSelection(tc.input, available)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got ids=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalInt64(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSelection_UnknownInRangeIsSilent(t *testing.T) {
	// Ring buffer has gaps because older segments rolled off.
	available := []int64{10, 11, 15}
	got, err := ParseSelection("10-20", available)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalInt64(got, available) {
		t.Fatalf("got %v, want %v", got, available)
	}
}

func TestParseSelection_OpenEndWithEmptyBufferErrors(t *testing.T) {
	if _, err := ParseSelection("5-", nil); err == nil {
		t.Fatal("expected error on open range with empty buffer")
	}
}

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
