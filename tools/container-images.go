package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
)

func getImageDigest(parentCtx context.Context, registryClient *regclient.RegClient, image string) (string, error) {
	r, err := ref.New(image)
	if err != nil {
		return "", errors.New("failed to create reference")
	}

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	manifest, err := registryClient.ManifestGet(ctx, r)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve manifest: %w", err)
	}

	return manifest.GetDescriptor().Digest.String(), nil
}
