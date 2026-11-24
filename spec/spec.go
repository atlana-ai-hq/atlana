// Package spec embute os JSON Schemas do formato Atlana, para que a CLI
// valide manifests contra exatamente o mesmo arquivo publicado em spec/.
package spec

import _ "embed"

// APIVersion é a versão de manifesto suportada por esta CLI.
const APIVersion = "atlana/v1alpha1"

// PackSchema é o JSON Schema de atlana.yaml.
//
//go:embed schemas/atlana.schema.json
var PackSchema []byte
