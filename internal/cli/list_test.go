package cli

import (
	"testing"

	"github.com/Kameleon21/oku/internal/model"
)

func TestListStatusFromArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		defaultList string
		want        model.Status
		wantErr     bool
	}{
		{name: "argument wins", args: []string{"finished"}, defaultList: "reading", want: model.StatusRead},
		{name: "invalid argument", args: []string{"bogus"}, defaultList: "reading", wantErr: true},
		{name: "falls back to default_list", defaultList: "oku", want: model.StatusWantToRead},
		{name: "default_list is trimmed and case-insensitive", defaultList: "  Paused ", want: model.StatusPaused},
		{name: "empty default_list", defaultList: "  ", wantErr: true},
		{name: "invalid default_list", defaultList: "shelf", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := listStatusFromArgs(tt.args, tt.defaultList)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("listStatusFromArgs(%v, %q) expected an error", tt.args, tt.defaultList)
				}
				return
			}
			if err != nil {
				t.Fatalf("listStatusFromArgs(%v, %q): %v", tt.args, tt.defaultList, err)
			}
			if got != tt.want {
				t.Fatalf("listStatusFromArgs(%v, %q) = %v, want %v", tt.args, tt.defaultList, got, tt.want)
			}
		})
	}
}

func TestValidateCount(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		max     int
		wantErr bool
	}{
		{name: "negative", value: -5, max: 52, wantErr: true},
		{name: "zero", value: 0, max: 52, wantErr: true},
		{name: "one", value: 1, max: 52},
		{name: "max", value: 52, max: 52},
		{name: "over max", value: 53, max: 52, wantErr: true},
		{name: "unbounded allows large values", value: 100000, max: 0},
		{name: "unbounded still rejects zero", value: 0, max: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCount("weeks", tt.value, tt.max)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validateCount(weeks, %d, %d) = %v, wantErr %v", tt.value, tt.max, err, tt.wantErr)
			}
		})
	}
}
