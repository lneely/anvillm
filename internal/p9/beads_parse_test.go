package p9

import (
	"testing"
)

func TestParseCtlCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantCmd string
		wantArgs []string
		wantErr bool
	}{
		{
			name:    "empty command",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "command only",
			input:   "claim",
			wantCmd: "claim",
			wantArgs: []string{},
		},
		{
			name:    "command with one arg",
			input:   "claim bd-123",
			wantCmd: "claim",
			wantArgs: []string{"bd-123"},
		},
		{
			name:    "command with multiple args",
			input:   "new title description parent",
			wantCmd: "new",
			wantArgs: []string{"title", "description", "parent"},
		},
		{
			name:    "single quoted arg",
			input:   "new 'my title'",
			wantCmd: "new",
			wantArgs: []string{"my title"},
		},
		{
			name:    "double quoted arg",
			input:   `new "my title"`,
			wantCmd: "new",
			wantArgs: []string{"my title"},
		},
		{
			name:    "mixed quoted and unquoted",
			input:   "new 'my title' description bd-parent",
			wantCmd: "new",
			wantArgs: []string{"my title", "description", "bd-parent"},
		},
		{
			name:    "empty quoted string",
			input:   "comment bd-123 ''",
			wantCmd: "comment",
			wantArgs: []string{"bd-123", ""},
		},
		{
			name:    "multiple spaces",
			input:   "claim    bd-123",
			wantCmd: "claim",
			wantArgs: []string{"bd-123"},
		},
		{
			name:    "tabs and spaces",
			input:   "claim\t\tbd-123",
			wantCmd: "claim",
			wantArgs: []string{"bd-123"},
		},
		{
			name:    "quoted arg with special chars",
			input:   "comment bd-123 'Work in progress: 50% done'",
			wantCmd: "comment",
			wantArgs: []string{"bd-123", "Work in progress: 50% done"},
		},
		{
			name:    "nested quotes not supported",
			input:   `new "title with 'nested' quotes"`,
			wantCmd: "new",
			wantArgs: []string{"title with ", "nested", " quotes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, err := parseCtlCommand(tt.input)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			if cmd != tt.wantCmd {
				t.Errorf("command = %q, want %q", cmd, tt.wantCmd)
			}
			
			if len(args) != len(tt.wantArgs) {
				t.Errorf("args length = %d, want %d\nargs = %v\nwant = %v", 
					len(args), len(tt.wantArgs), args, tt.wantArgs)
				return
			}
			
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}
