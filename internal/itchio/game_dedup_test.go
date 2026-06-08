package itchio

import "testing"

func TestDeduplicateUploadsByFilename(t *testing.T) {
	tests := []struct {
		name          string
		input         []Upload
		wantFilenames []string
		wantIDs       []string
	}{
		{
			name: "no duplicates passes through unchanged",
			input: []Upload{
				{Filename: "game.gb", UploadID: "100"},
				{Filename: "game.gba", UploadID: "200"},
			},
			wantFilenames: []string{"game.gb", "game.gba"},
			wantIDs:       []string{"100", "200"},
		},
		{
			name: "duplicate keeps highest ID (real-world Crash Bandicoot case)",
			input: []Upload{
				{Filename: "crash_bandicoot.gba", UploadID: "15902350"},
				{Filename: "crash_bandicoot.gba", UploadID: "15887634"},
			},
			wantFilenames: []string{"crash_bandicoot.gba"},
			wantIDs:       []string{"15902350"},
		},
		{
			name: "duplicate keeps highest ID when lower comes first",
			input: []Upload{
				{Filename: "game.gba", UploadID: "100"},
				{Filename: "game.gba", UploadID: "200"},
			},
			wantFilenames: []string{"game.gba"},
			wantIDs:       []string{"200"},
		},
		{
			name: "multiple filenames each with a duplicate",
			input: []Upload{
				{Filename: "a.gb", UploadID: "1"},
				{Filename: "b.gbc", UploadID: "2"},
				{Filename: "a.gb", UploadID: "3"},
				{Filename: "b.gbc", UploadID: "4"},
			},
			wantFilenames: []string{"a.gb", "b.gbc"},
			wantIDs:       []string{"3", "4"},
		},
		{
			name: "mixed: some unique some duplicated",
			input: []Upload{
				{Filename: "game.gb", UploadID: "10"},
				{Filename: "extra.gba", UploadID: "20"},
				{Filename: "game.gb", UploadID: "30"},
			},
			wantFilenames: []string{"extra.gba", "game.gb"},
			wantIDs:       []string{"20", "30"},
		},
		{
			name:          "empty input",
			input:         []Upload{},
			wantFilenames: []string{},
			wantIDs:       []string{},
		},
		{
			name: "single entry unchanged",
			input: []Upload{
				{Filename: "only.gba", UploadID: "999"},
			},
			wantFilenames: []string{"only.gba"},
			wantIDs:       []string{"999"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateUploadsByFilename(tt.input)
			if len(got) != len(tt.wantFilenames) {
				t.Fatalf("got %d uploads, want %d\ngot: %+v", len(got), len(tt.wantFilenames), got)
			}
			// Build a map of filename→ID from the result for order-independent checking.
			byName := make(map[string]string, len(got))
			for _, u := range got {
				byName[u.Filename] = u.UploadID
			}
			for i, fn := range tt.wantFilenames {
				wantID := tt.wantIDs[i]
				if gotID, ok := byName[fn]; !ok {
					t.Errorf("filename %q missing from result", fn)
				} else if gotID != wantID {
					t.Errorf("filename %q: UploadID = %q, want %q", fn, gotID, wantID)
				}
			}
		})
	}
}
