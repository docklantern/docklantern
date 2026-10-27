// Package buildinfo exposes the version baked into the running binary.
// It is distinct from Docker's container creation image reference (Config.Image).
// Release builds set Version through a link-time -X flag; local builds use dev.
package buildinfo

var Version = "dev"
