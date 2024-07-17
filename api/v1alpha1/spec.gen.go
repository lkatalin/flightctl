// Package v1alpha1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.3.0 DO NOT EDIT.
package v1alpha1

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %w", err)
	}

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	res := make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	resolvePath := PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		pathToFile := url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
