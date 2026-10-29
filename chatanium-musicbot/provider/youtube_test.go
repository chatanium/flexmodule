package provider

import (
	"context"
	"testing"
)

func TestMetadataOrderBoundAndPartialFailure(t *testing.T) {
	first := `{"id":"first","title":"first title","url":"https://media.example/one"}`
	second := `{"id":"second","title":"second title","url":"https://media.example/two"}`
	tracks, err := parseTracks([]byte(first+"\n"+second), 2)
	if err != nil || len(tracks) != 2 || tracks[0].Title != "first title" || tracks[1].Title != "second title" {
		t.Fatalf("provider order: %+v %v", tracks, err)
	}
	if tracks[0].ID == "first" || tracks[0].ID == tracks[1].ID {
		t.Fatal("external ID treated as opaque artifact handle")
	}
	for _, raw := range []string{"", first + "\n{", first + "\n" + second + "\n" + first, `{"id":"x","title":"x","url":"file:///etc/passwd"}`} {
		if partial, err := parseTracks([]byte(raw), 2); err == nil || partial != nil {
			t.Fatalf("partial/empty/unbounded response published: %+v %v", partial, err)
		}
	}
}
func TestProcessOutputLimitCancelsInsteadOfDiscardingExcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := &cappedOutput{remaining: 3, cancel: cancel}
	if n, err := output.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatal(n, err)
	}
	if _, err := output.Write([]byte("d")); err == nil || !output.exceeded || ctx.Err() == nil {
		t.Fatal("overflow was hidden or did not cancel owned process")
	}
	if output.data.String() != "abc" {
		t.Fatal("excess process output retained")
	}
}
