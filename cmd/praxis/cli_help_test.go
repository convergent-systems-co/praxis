package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunInterceptsNestedHelpBeforeCommandDispatch(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	err = run([]string{"publisher", "enroll", "--help"})
	_ = write.Close()
	os.Stdout = original
	body, readErr := io.ReadAll(read)
	_ = read.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(string(body), "usage: praxis publisher enroll --approval <digest>") {
		t.Fatalf("run output %q", body)
	}
}

func TestCLIHelpContractAcrossCommandHierarchy(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"root flag", []string{"--help"}, "usage: praxis <command|installed-entry-point>"},
		{"root help", []string{"help"}, "usage: praxis <command|installed-entry-point>"},
		{"parent flag", []string{"publisher", "--help"}, "usage: praxis publisher <key-create"},
		{"parent help", []string{"help", "publisher"}, "usage: praxis publisher <key-create"},
		{"leaf flag", []string{"publisher", "enroll", "--help"}, "usage: praxis publisher enroll --approval <digest>"},
		{"leaf help", []string{"help", "publisher", "enroll"}, "usage: praxis publisher enroll --approval <digest>"},
		{"other family", []string{"migration", "execute", "--help"}, "usage: praxis migration execute --preview-file <path>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			handled, err := dispatchCLIHelp(tt.args, &out)
			if !handled || err != nil {
				t.Fatalf("help dispatch = handled %v, err %v", handled, err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("help output %q does not contain %q", out.String(), tt.want)
			}
			if strings.Contains(out.String(), "flag: help requested") {
				t.Fatal("flag parser error leaked into help output")
			}
		})
	}
}

func TestCLIHelpRejectsUnknownCommandsAtTheCorrectHierarchy(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown root", []string{"missing", "--help"}, "unknown command"},
		{"unknown publisher child", []string{"publisher", "status", "--help"}, "unknown command \"publisher status\""},
		{"unknown migration child", []string{"help", "migration", "missing"}, "unknown command \"migration missing\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			handled, err := dispatchCLIHelp(tt.args, &out)
			if !handled || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("help dispatch = handled %v, err %v; want %q", handled, err, tt.want)
			}
			if tt.name == "unknown publisher child" && !strings.Contains(out.String(), "usage: praxis publisher") {
				t.Fatalf("unknown child did not show parent commands: %q", out.String())
			}
			if strings.Contains(out.String(), "flag: help requested") {
				t.Fatal("flag parser error leaked for unknown command")
			}
		})
	}
}

func TestCLIHelpDoesNotInterceptOrdinaryArguments(t *testing.T) {
	handled, err := dispatchCLIHelp([]string{"publisher", "enroll", "--approval", "digest"}, &bytes.Buffer{})
	if handled || err != nil {
		t.Fatalf("ordinary command was treated as help: handled=%v err=%v", handled, err)
	}
}
