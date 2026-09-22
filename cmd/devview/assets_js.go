//go:build js

package main

import (
	"fmt"
	"io"
	"net/http"
)

// Where the pictures come from in a browser (TODO 10): off the network,
// beside the page, by their hashed names.
//
// Fetched rather than embedded, which is the whole point of the hash. The
// wasm file is 15 MB and changes whenever a rule does; the sheet is 1.4 kB
// and changes when somebody draws. Carrying the art inside the program would
// mean re-downloading the pictures every time a rule moved, and caching the
// pictures for ever would mean never seeing a new one - the hash in the name
// settles both at once.
//
// And here, unlike the build with a filesystem behind it, only the set being
// drawn with is ever fetched: a name is a directory under assets/, so the
// sets nobody asked for cost the telephone nothing.
func loadAsset(name string) ([]byte, error) {
	resp, err := http.Get("assets/" + name)
	if err != nil {
		return nil, fmt.Errorf("asset %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset %s: %s", name, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("asset %s: %w", name, err)
	}
	return b, nil
}
