package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runKeyBootstrap(args []string) error {
	flags := flag.NewFlagSet("key-bootstrap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "", "explicit bootstrap provider")
	keyID := flags.String("key-id", "", "opaque Praxis key identity")
	owner := flags.String("owner", "", "bootstrap owner identity")
	purpose := flags.String("purpose", "", "key purpose")
	profile := flags.String("profile", "", "crypto profile")
	output := flags.String("output", "", "metadata-only bootstrap record path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("key-bootstrap does not accept positional arguments")
	}
	selected, err := parseCryptoProfile(*profile)
	if err != nil {
		return err
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return fmt.Errorf("construct first-party bootstrap registry: %w", err)
	}
	record, _, err := registry.Bootstrap(context.Background(), praxiscrypto.BootstrapRequest{ProviderID: *provider, KeyID: *keyID, Owner: *owner, Purpose: *purpose, Profile: selected})
	if err != nil {
		return fmt.Errorf("bootstrap key provider: %w", err)
	}
	if err := praxiscrypto.SaveBootstrapRecord(*output, record); err != nil {
		return fmt.Errorf("persist bootstrap metadata: %w", err)
	}
	digest, err := record.Digest()
	if err != nil {
		return err
	}
	fmt.Printf("bootstrap record created: provider=%s key_id=%s key_version=%s digest=%s path=%s\n", record.ProviderID, record.KeyID, record.KeyVersion, digest, *output)
	return nil
}

func parseCryptoProfile(value string) (contracts.CryptoProfile, error) {
	profile := contracts.CryptoProfile(value)
	if err := profile.Validate(); err != nil {
		return "", fmt.Errorf("profile is required and must be valid: %w", err)
	}
	return profile, nil
}
