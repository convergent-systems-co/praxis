//go:build !darwin

package crypto

import "context"

type UnsupportedPublisherBackend struct{}

func (UnsupportedPublisherBackend) Generate(context.Context, string) (PublisherSigner, error) {
	return nil, ErrPublisherSigningUnavailable
}
func (UnsupportedPublisherBackend) Open(context.Context, string) (PublisherSigner, error) {
	return nil, ErrPublisherSigningUnavailable
}
func (UnsupportedPublisherBackend) Delete(context.Context, string) error {
	return ErrPublisherSigningUnavailable
}
