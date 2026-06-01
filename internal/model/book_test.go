package model

import (
	"testing"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		input   string
		wantAbs int
		wantDel int
		wantRel bool
		wantErr bool
	}{
		{"123", 123, 0, false, false},
		{"0", 0, 0, false, false},
		{"+10", 0, 10, true, false},
		{"-5", 0, -5, true, false},
		{"+0", 0, 0, true, false},
		{"", 0, 0, false, true},
		{"abc", 0, 0, false, true},
		{"+abc", 0, 0, false, true},
		{"-", 0, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePage(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePage(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParsePage(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got.Absolute != tt.wantAbs || got.Delta != tt.wantDel || got.Relative != tt.wantRel {
				t.Errorf("ParsePage(%q) = {Abs:%d, Del:%d, Rel:%v}, want {Abs:%d, Del:%d, Rel:%v}",
					tt.input, got.Absolute, got.Delta, got.Relative, tt.wantAbs, tt.wantDel, tt.wantRel)
			}
		})
	}
}

func TestPageUpdateResolve(t *testing.T) {
	tests := []struct {
		name    string
		update  PageUpdate
		current int
		total   int
		want    int
	}{
		{"absolute", PageUpdate{Absolute: 50}, 10, 300, 50},
		{"delta positive", PageUpdate{Delta: 10, Relative: true}, 50, 300, 60},
		{"delta negative", PageUpdate{Delta: -5, Relative: true}, 50, 300, 45},
		{"clamp to zero", PageUpdate{Delta: -100, Relative: true}, 50, 300, 0},
		{"clamp to total", PageUpdate{Absolute: 999}, 50, 300, 300},
		{"no total, no clamp", PageUpdate{Absolute: 999}, 50, 0, 999},
		{"delta beyond total", PageUpdate{Delta: 500, Relative: true}, 50, 300, 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.update.Resolve(tt.current, tt.total)
			if got != tt.want {
				t.Errorf("Resolve() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStatusFromString(t *testing.T) {
	tests := []struct {
		input   string
		want    Status
		wantErr bool
	}{
		{"reading", StatusCurrentlyReading, false},
		{"oku", StatusWantToRead, false},
		{"want", StatusWantToRead, false},
		{"wtr", StatusWantToRead, false},
		{"want-to-read", StatusWantToRead, false},
		{"finished", StatusRead, false},
		{"read", StatusRead, false},
		{"done", StatusRead, false},
		{"dnf", StatusDidNotFinish, false},
		{"Reading", StatusCurrentlyReading, false},
		{"READING", StatusCurrentlyReading, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := StatusFromString(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("StatusFromString(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("StatusFromString(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("StatusFromString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusWantToRead, "oku"},
		{StatusCurrentlyReading, "reading"},
		{StatusRead, "finished"},
		{StatusDidNotFinish, "dnf"},
		{StatusPaused, "paused"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status(%d).String() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestValidateRating(t *testing.T) {
	tests := []struct {
		name    string
		rating  float64
		wantErr bool
	}{
		{name: "zero unrated", rating: 0, wantErr: false},
		{name: "half", rating: 0.5, wantErr: false},
		{name: "whole", rating: 4, wantErr: false},
		{name: "max", rating: 5, wantErr: false},
		{name: "quarter invalid", rating: 4.25, wantErr: true},
		{name: "too high", rating: 5.5, wantErr: true},
		{name: "negative", rating: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRating(tt.rating)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateRating(%v) expected error", tt.rating)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateRating(%v) unexpected error: %v", tt.rating, err)
			}
		})
	}
}

func TestStarString(t *testing.T) {
	tests := []struct {
		rating float64
		want   string
	}{
		{rating: 0, want: "☆☆☆☆☆"},
		{rating: 4, want: "★★★★☆"},
		{rating: 4.5, want: "★★★★½"},
		{rating: 5, want: "★★★★★"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := StarString(tt.rating)
			if got != tt.want {
				t.Fatalf("StarString(%v) = %q, want %q", tt.rating, got, tt.want)
			}
		})
	}
}
