package worktime

import (
	"errors"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal/config"
)

func testAccountingConfig() config.AccountingConfig {
	return config.Default().Accounting
}

func TestClassifyTag(t *testing.T) {
	cfg := testAccountingConfig()

	tests := []struct {
		name string
		tag  string
		want TagClass
	}{
		{name: "work", tag: "work", want: TagClassWork},
		{name: "plusfor off", tag: "off", want: TagClassPlus},
		{name: "plusfor bank", tag: "bank", want: TagClassPlus},
		{name: "minusfor lunch", tag: "lunch", want: TagClassMinus},
		{name: "bufferfor selfdevelopment", tag: "selfdevelopment", want: TagClassBuffer},
		{name: "bufferfor pet", tag: "pet", want: TagClassBuffer},
		{name: "free label blogpost", tag: "blogpost", want: TagClassLabel},
		{name: "legacy label dotfiles", tag: "dotfiles", want: TagClassLabel},
		{name: "trimmed", tag: "  work  ", want: TagClassWork},
		{name: "empty", tag: "", want: TagClassLabel},
		{name: "whitespace only", tag: "   ", want: TagClassLabel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyTag(cfg, tt.tag)
			if got != tt.want {
				t.Fatalf("ClassifyTag(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestAccountingTag(t *testing.T) {
	cfg := testAccountingConfig()

	tests := []struct {
		name      string
		tags      []string
		want      string
		wantErr   bool
		errIsMult bool
	}{
		{
			name: "work only",
			tags: []string{"work"},
			want: "work",
		},
		{
			name: "work with buffer and label tags",
			tags: []string{"work", "selfdevelopment", "blogpost"},
			want: "work",
		},
		{
			name: "primary plus label",
			tags: []string{"off", "meeting"},
			want: "off",
		},
		{
			name: "buffer only",
			tags: []string{"selfdevelopment"},
			want: "selfdevelopment",
		},
		{
			name: "buffer with free label",
			tags: []string{"selfdevelopment", "blogpost"},
			want: "selfdevelopment",
		},
		{
			name: "labels only",
			tags: []string{"blogpost", "dotfiles"},
			want: "",
		},
		{
			name:      "two plusfor tags",
			tags:      []string{"off", "bank"},
			wantErr:   true,
			errIsMult: true,
		},
		{
			name:      "work then off",
			tags:      []string{"work", "off"},
			wantErr:   true,
			errIsMult: true,
		},
		{
			name:      "two buffer tags without primary",
			tags:      []string{"selfdevelopment", "tools"},
			wantErr:   true,
			errIsMult: true,
		},
		{
			name:    "duplicate tag",
			tags:    []string{"work", "work"},
			wantErr: true,
		},
		{
			name:    "empty tag element",
			tags:    []string{"work", ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AccountingTag(cfg, tt.tags)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errIsMult && !errors.Is(err, ErrMultipleAccountingTags) {
					t.Fatalf("expected ErrMultipleAccountingTags, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("AccountingTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTags(t *testing.T) {
	cfg := testAccountingConfig()

	tests := []struct {
		name    string
		tags    []string
		wantErr bool
	}{
		{name: "accept work selfdevelopment blogpost", tags: []string{"work", "selfdevelopment", "blogpost"}},
		{name: "reject off bank", tags: []string{"off", "bank"}, wantErr: true},
		{name: "empty tags ok", tags: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTags(cfg, tt.tags)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateEntry(t *testing.T) {
	cfg := testAccountingConfig()

	valid := Entry{
		ID:     1,
		Host:   "earth",
		Action: "add",
		Epoch:  1787951450,
		Value:  7200,
		Descr:  "Wrote up the observability post",
		Tags:   []string{"work", "blogpost"},
	}

	tests := []struct {
		name    string
		entry   Entry
		wantErr string
	}{
		{name: "valid add", entry: valid},
		{
			name: "valid login",
			entry: Entry{
				ID: 2, Host: "earth", Action: "login", Epoch: 1787917475, Tags: []string{"work"},
			},
		},
		{
			name: "invalid id",
			entry: func() Entry {
				e := valid
				e.ID = 0
				return e
			}(),
			wantErr: "entry id must be positive",
		},
		{
			name: "missing host",
			entry: func() Entry {
				e := valid
				e.Host = "  "
				return e
			}(),
			wantErr: "entry host must not be empty",
		},
		{
			name: "unsupported action",
			entry: func() Entry {
				e := valid
				e.Action = "pause"
				return e
			}(),
			wantErr: `unsupported action "pause"`,
		},
		{
			name: "non-positive epoch",
			entry: func() Entry {
				e := valid
				e.Epoch = 0
				return e
			}(),
			wantErr: "entry epoch must be positive",
		},
		{
			name: "login with value",
			entry: Entry{
				ID: 3, Host: "earth", Action: "login", Epoch: 1, Value: 60, Tags: []string{"work"},
			},
			wantErr: `action "login" must not carry value`,
		},
		{
			name: "two accounting tags",
			entry: func() Entry {
				e := valid
				e.Tags = []string{"off", "bank"}
				return e
			}(),
			wantErr: "multiple accounting tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEntry(cfg, tt.entry)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
