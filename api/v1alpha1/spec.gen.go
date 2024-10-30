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
	"AA==",
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
