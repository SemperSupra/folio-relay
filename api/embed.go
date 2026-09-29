package api

import _ "embed"

// OpenAPI is the product API contract consumed by humans, automation, agents,
// CI, and HIL validators.
//
//go:embed openapi.yaml
var OpenAPI []byte
