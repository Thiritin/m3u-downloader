package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertCategories(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cats := []CategoryRow{
		{ID: 1, Type: "vod", Name: "Action"},
		{ID: 2, Type: "vod", Name: "Drama"},
	}
	if err := s.UpsertCategories(ctx, cats); err != nil { t.Fatal(err) }
	got, err := s.ListCategories(ctx, "vod")
	if err != nil { t.Fatal(err) }
	if len(got) != 2 { t.Errorf("got %d categories, want 2", len(got)) }
	cats[0].Name = "Action!"
	if err := s.UpsertCategories(ctx, cats); err != nil { t.Fatal(err) }
	got, _ = s.ListCategories(ctx, "vod")
	if len(got) != 2 || got[0].Name != "Action!" {
		t.Errorf("upsert did not update: %+v", got)
	}
}

func TestUpsertVODs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.UpsertCategories(ctx, []CategoryRow{{ID: 1, Type: "vod", Name: "A"}}); err != nil {
		t.Fatal(err)
	}
	vods := []VODRow{
		{StreamID: 100, CategoryID: 1, Name: "M1", Year: 2020, ContainerExt: "mkv"},
		{StreamID: 101, CategoryID: 1, Name: "M2", Year: 2021, ContainerExt: "ts"},
	}
	if err := s.UpsertVODs(ctx, vods); err != nil { t.Fatal(err) }
	got, err := s.ListVODs(ctx, 1)
	if err != nil { t.Fatal(err) }
	if len(got) != 2 { t.Errorf("got %d vods, want 2", len(got)) }
}
