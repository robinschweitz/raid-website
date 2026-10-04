// Package catalogdata embeds the single catalog asset without runtime file access.
package catalogdata

import _ "embed"

//go:embed catalog-wotlk335.json
var bundled string

// JSON returns the immutable, build-time catalog source.
func JSON() string { return bundled }
