// Package spec embute os JSON Schemas do formato Atlana, para que a CLI
// valide manifests contra exatamente o mesmo arquivo publicado em spec/.
package spec

import _ "embed"

// APIVersion é a versão de manifesto atual (ADR 0008).
const APIVersion = "atlana/v1"

// LegacyAPIVersion ainda é aceita, com aviso, para packs anteriores à v1.
const LegacyAPIVersion = "atlana/v1alpha1"

// Supported diz se a CLI lê manifestos com essa apiVersion.
func Supported(v string) bool { return v == APIVersion || v == LegacyAPIVersion }

// PackSchema é o JSON Schema de atlana.yaml.
//
//go:embed schemas/atlana.schema.json
var PackSchema []byte
