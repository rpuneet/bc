package cmd

import "testing"

func TestUpCmd_DefaultAddr(t *testing.T) {
	f := upCmd.Flags().Lookup("addr")
	if f == nil {
		t.Fatal("addr flag not found")
	}
	if f.DefValue != "127.0.0.1:9374" {
		t.Errorf("got %q, want %q", f.DefValue, "127.0.0.1:9374")
	}
}

func TestNormalizeAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{":8080", "127.0.0.1:8080"},
		{":9374", "127.0.0.1:9374"},
		{"0.0.0.0:9374", "0.0.0.0:9374"},
		{"127.0.0.1:9374", "127.0.0.1:9374"},
		{"localhost:9374", "localhost:9374"},
		{"notaport", "notaport"}, // no host:port → returned as-is
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeAddr(tt.input)
			if got != tt.want {
				t.Errorf("normalizeAddr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
