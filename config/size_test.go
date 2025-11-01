package config

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "1GB",
			input: "1GB",
			want:  1024 * 1024 * 1024,
		},
		{
			name:  "512MB",
			input: "512MB",
			want:  512 * 1024 * 1024,
		},
		{
			name:  "16GB",
			input: "16GB",
			want:  16 * 1024 * 1024 * 1024,
		},
		{
			name:  "100MB",
			input: "100MB",
			want:  100 * 1024 * 1024,
		},
		{
			name:  "4KB",
			input: "4KB",
			want:  4 * 1024,
		},
		{
			name:  "lowercase gb",
			input: "1gb",
			want:  1024 * 1024 * 1024,
		},
		{
			name:  "with spaces",
			input: " 2GB ",
			want:  2 * 1024 * 1024 * 1024,
		},
		{
			name:  "decimal",
			input: "1.5GB",
			want:  int(1.5 * 1024 * 1024 * 1024),
		},
		{
			name:  "empty defaults to 1GB",
			input: "",
			want:  1024 * 1024 * 1024,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			input:   "1XB",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
