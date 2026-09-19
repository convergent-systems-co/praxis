package client

import (
	"errors"
	"fmt"
	"strings"
)

type Invocation struct {
	Namespace  string
	EntryPoint string
	Arguments  []string
	Options    map[string]string
}

// ParseSlashInvocation normalizes the discoverable client UX into a canonical
// structure. It deliberately does not authorize any resulting operation.
func ParseSlashInvocation(input string) (Invocation, error) {
	return ParseInvocationFields(strings.Fields(strings.TrimSpace(input)))
}

// ParseInvocationFields normalizes an already-split invocation, such as a
// process argv, without re-splitting any element: an option value is the
// whole element after "=", whitespace included (#159).
func ParseInvocationFields(fields []string) (Invocation, error) {
	if len(fields) < 2 {
		return Invocation{}, errors.New("expected /praxis <entry-point>")
	}
	if fields[0] != "/praxis" && fields[0] != "praxis" {
		return Invocation{}, errors.New("invocation must use praxis namespace")
	}
	inv := Invocation{Namespace: "praxis", EntryPoint: fields[1], Options: map[string]string{}}
	if strings.HasPrefix(inv.EntryPoint, "-") || inv.EntryPoint == "" {
		return Invocation{}, errors.New("entry point is required before options")
	}
	for i := 2; i < len(fields); i++ {
		field := fields[i]
		if !strings.HasPrefix(field, "--") {
			inv.Arguments = append(inv.Arguments, field)
			continue
		}
		nameValue := strings.TrimPrefix(field, "--")
		if nameValue == "" {
			return Invocation{}, errors.New("empty option name")
		}
		name, value, found := strings.Cut(nameValue, "=")
		if !found {
			value = "true"
		}
		if name == "" {
			return Invocation{}, errors.New("empty option name")
		}
		if _, exists := inv.Options[name]; exists {
			return Invocation{}, fmt.Errorf("duplicate option %q", name)
		}
		inv.Options[name] = value
	}
	return inv, nil
}
