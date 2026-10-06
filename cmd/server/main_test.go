package main

import (
	"testing"

	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
)

func TestDetectPattern(t *testing.T) {
	for _, tc := range []struct {
		name     string
		series   *raidseries.Series
		raids    []raid.Raid
		want     bool
		template string
		nextNum  int
	}{
		{
			name:     "empty_raids",
			raids:    nil,
			want:     false,
			template: "",
			nextNum:  0,
		},
		{
			name: "explicitly_configured_template_is_preserved",
			series: &raidseries.Series{
				Name: "ICC Montagsraid", NameTemplate: "custom-{name}-{number}", NextRaidNumber: 7,
			},
			raids: []raid.Raid{
				{Name: "icc25-01"},
				{Name: "icc25-33"},
			},
			want:     false,
			template: "custom-{name}-{number}",
			nextNum:  7,
		},
		{
			name: "template_equal_to_name_is_detected",
			series: &raidseries.Series{
				Name: "ICC25-01", NameTemplate: "ICC25-01", NextRaidNumber: 1,
			},
			raids: []raid.Raid{
				{Name: "ICC25-01"},
				{Name: "ICC25-33"},
			},
			want:     true,
			template: "ICC25-{number}",
			nextNum:  34,
		},
		{
			name: "sequential_numbering",
			raids: []raid.Raid{
				{Name: "icc25-01"},
				{Name: "icc25-02"},
				{Name: "icc25-33"},
			},
			want:     true,
			template: "icc25-{number}",
			nextNum:  34,
		},
		{
			name: "single_raid",
			raids: []raid.Raid{
				{Name: "icc25-01"},
			},
			want:     true,
			template: "icc25-{number}",
			nextNum:  2,
		},
		{
			name: "inconsistent_prefix",
			raids: []raid.Raid{
				{Name: "icc25-01"},
				{Name: "icc10-02"},
			},
			want: false,
		},
		{
			name: "non_numeric_suffix",
			raids: []raid.Raid{
				{Name: "icc25-alpha"},
			},
			want: false,
		},
		{
			name: "no_dash_separator",
			raids: []raid.Raid{
				{Name: "icc2501"},
			},
			want: false,
		},
		{
			name: "zero_number",
			raids: []raid.Raid{
				{Name: "icc25-00"},
			},
			want: false,
		},
		{
			name: "underscore_delimiter_not_detected",
			raids: []raid.Raid{
				{Name: "icc25_01"},
				{Name: "icc25_02"},
			},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.series
			if s == nil {
				s = &raidseries.Series{}
			}
			got := detectPattern(s, tc.raids)
			if got != tc.want {
				t.Fatalf("want %v got %v", tc.want, got)
			}
			if tc.want || tc.series != nil {
				if s.NameTemplate != tc.template {
					t.Fatalf("want template %q got %q", tc.template, s.NameTemplate)
				}
				if s.NextRaidNumber != tc.nextNum {
					t.Fatalf("want next number %d got %d", tc.nextNum, s.NextRaidNumber)
				}
			}
		})
	}
}
