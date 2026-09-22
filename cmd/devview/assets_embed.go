//go:build !js

package main

import (
	"embed"
	"fmt"
)

// Where the pictures come from on a machine with a filesystem (TODO 10):
// inside the program. There is no network here and no install step, so a
// development build has to carry its own art - the same reason the golden
// test carries its own numbers.
//
// All of the art, not one set of it. A set is a few hundred kilobytes and
// this binary is tens of megabytes, so carrying every set costs nothing and
// buys the one thing the sets are for: switching between them without
// rebuilding.
//
//go:embed assets
var assets embed.FS

func loadAsset(name string) ([]byte, error) {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return nil, fmt.Errorf("asset %s: %w", name, err)
	}
	return b, nil
}
