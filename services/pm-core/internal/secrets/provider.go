// Package secrets is the single door through which pm-core reads secrets.
//
// The FSD (§05, §11) requires that code never knows where a secret came from:
// in development that is a gitignored .env file, in production a vault. Every
// backend implements Provider; Chain tries them in order. Adding a Vault or KMS
// provider later is one new file and one line in FromEnvironment.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNotFound is returned when no provider in the chain holds the named secret.
var ErrNotFound = errors.New("secret not found")

// Provider resolves a secret by name.
type Provider interface {
	// Get returns the secret value, or ErrNotFound (possibly wrapped).
	Get(ctx context.Context, name string) (string, error)
	// Name identifies the provider in logs. Never includes secret material.
	Name() string
}

// Chain tries each provider in order and returns the first hit.
type Chain []Provider

func (c Chain) Get(ctx context.Context, name string) (string, error) {
	for _, p := range c {
		v, err := p.Get(ctx, name)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("secrets: %s: %w", p.Name(), err)
		}
	}
	return "", fmt.Errorf("secrets: %q: %w", name, ErrNotFound)
}

func (c Chain) Name() string {
	names := make([]string, 0, len(c))
	for _, p := range c {
		names = append(names, p.Name())
	}
	return "chain(" + strings.Join(names, ",") + ")"
}

// Env reads secrets from process environment variables.
type Env struct{}

func (Env) Get(_ context.Context, name string) (string, error) {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v, nil
	}
	return "", ErrNotFound
}

func (Env) Name() string { return "env" }

// FromEnvironment builds the default chain for this process: a secrets file
// when PM_SECRETS_FILE is set (development), then the environment.
func FromEnvironment() (Provider, error) {
	var chain Chain
	if path := os.Getenv("PM_SECRETS_FILE"); path != "" {
		fp, err := NewFile(path)
		if err != nil {
			return nil, err
		}
		chain = append(chain, fp)
	}
	chain = append(chain, Env{})
	return chain, nil
}
