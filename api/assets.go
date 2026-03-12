// Package api provides embedded API assets (OpenAPI spec and Swagger UI).
package api

import "embed"

// OpenAPISpec is the raw OpenAPI 3.0 YAML specification.
//
//go:embed openapi.yaml
var OpenAPISpec []byte

// SwaggerUIFS contains the Swagger UI static files rooted at the "swagger-ui" directory.
//
//go:embed swagger-ui
var SwaggerUIFS embed.FS
