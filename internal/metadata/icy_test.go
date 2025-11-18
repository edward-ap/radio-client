package metadata

import "testing"

func TestExtractStreamTitle(t *testing.T) {
	tests := []struct {
		name string
		meta string
		want string
	}{
		{
			name: "single quotes simple",
			meta: "StreamTitle='Artist - Track';",
			want: "Artist - Track",
		},
		{
			name: "quoted apostrophe",
			meta: "StreamTitle='JANE'S ADDICTION - BEEN CAUGHT STEALING';",
			want: "JANE'S ADDICTION - BEEN CAUGHT STEALING",
		},
		{
			name: "double quotes",
			meta: `StreamTitle="Double Quoted Title";`,
			want: "Double Quoted Title",
		},
		{
			name: "missing terminator uses entire tail",
			meta: "StreamTitle='No Terminator",
			want: "No Terminator",
		},
		{
			name: "trim spaces and HTML entities",
			meta: "StreamTitle=' AC/DC &amp; Friends ';",
			want: "AC/DC & Friends",
		},
		{
			name: "empty result",
			meta: "StreamTitle='';",
			want: "",
		},
		{
			name: "no stream title present",
			meta: "StreamUrl='http://example'",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractStreamTitle(tt.meta); got != tt.want {
				t.Fatalf("extractStreamTitle(%q) = %q, want %q", tt.meta, got, tt.want)
			}
		})
	}
}
