package model

import "testing"

func TestTagsForCategory(t *testing.T) {
	raw := `{"Genre":[{"tag":"Fantasy"},{"tag":"Classics"},{"tag":""}],"Mood":[{"tag":"dark"}]}`

	genres := TagsForCategory(raw, "Genre")
	if len(genres) != 2 || genres[0] != "Fantasy" || genres[1] != "Classics" {
		t.Fatalf("genres = %v, want [Fantasy Classics]", genres)
	}
	if moods := TagsForCategory(raw, "Mood"); len(moods) != 1 || moods[0] != "dark" {
		t.Fatalf("moods = %v, want [dark]", moods)
	}
	if got := TagsForCategory(raw, "Missing"); got != nil {
		t.Fatalf("missing category = %v, want nil", got)
	}
	if got := TagsForCategory("", "Genre"); got != nil {
		t.Fatalf("empty blob = %v, want nil", got)
	}
	if got := TagsForCategory("{not json", "Genre"); got != nil {
		t.Fatalf("malformed blob = %v, want nil", got)
	}
}

func TestGoalPercent(t *testing.T) {
	cases := []struct {
		goal Goal
		want int
	}{
		{Goal{Target: 20, Progress: 12}, 60},
		{Goal{Target: 20, Progress: 25}, 100}, // capped
		{Goal{Target: 0, Progress: 5}, 0},     // no target
		{Goal{Target: 3, Progress: 1}, 33},
	}
	for _, c := range cases {
		if got := c.goal.Percent(); got != c.want {
			t.Errorf("Percent(%+v) = %d, want %d", c.goal, got, c.want)
		}
	}
}
