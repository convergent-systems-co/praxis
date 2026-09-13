package plugin

import (
	"errors"
	"fmt"
	"strconv"
)

var ErrProtocolIncompatible = errors.New("plugin protocol range is incompatible")

type ProtocolRange struct {
	Min string
	Max string
}

func (r ProtocolRange) Validate() error {
	min, err := parseProtocolVersion(r.Min)
	if err != nil { return fmt.Errorf("minimum protocol version: %w", err) }
	max, err := parseProtocolVersion(r.Max)
	if err != nil { return fmt.Errorf("maximum protocol version: %w", err) }
	if min > max { return errors.New("protocol minimum exceeds maximum") }
	return nil
}

// NegotiateProtocol returns the highest mutually supported protocol version.
// Protocol versions are intentionally monotonic integer contract generations;
// feature negotiation belongs in capability/property discovery, not version strings.
func NegotiateProtocol(core, plugin ProtocolRange) (string, error) {
	if err := core.Validate(); err != nil { return "", fmt.Errorf("core protocol range: %w", err) }
	if err := plugin.Validate(); err != nil { return "", fmt.Errorf("plugin protocol range: %w", err) }
	coreMin, _ := parseProtocolVersion(core.Min)
	coreMax, _ := parseProtocolVersion(core.Max)
	pluginMin, _ := parseProtocolVersion(plugin.Min)
	pluginMax, _ := parseProtocolVersion(plugin.Max)
	low := coreMin
	if pluginMin > low { low = pluginMin }
	high := coreMax
	if pluginMax < high { high = pluginMax }
	if low > high { return "", ErrProtocolIncompatible }
	return strconv.FormatUint(high, 10), nil
}

func parseProtocolVersion(v string) (uint64, error) {
	if v == "" { return 0, errors.New("protocol version is required") }
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil || n == 0 { return 0, fmt.Errorf("invalid protocol version %q", v) }
	return n, nil
}
