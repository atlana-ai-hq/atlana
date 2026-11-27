// Package configmerge edita, sem destruir o resto, a seção de servidores MCP de
// arquivos de configuração de agentes (JSON e TOML).
//
// Só a seção pedida (ex.: "mcpServers") muda; as outras chaves de topo ficam na
// ordem e com o valor que tinham. A propriedade de cada servidor é guardada no
// atlana.lock com o hash de Canon, nunca dentro do arquivo do agente.
package configmerge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Format é o formato do arquivo de configuração.
type Format string

const (
	JSON Format = "json"
	TOML Format = "toml"
)

// Doc é um arquivo de configuração aberto para edição de uma seção de servidores.
type Doc interface {
	// Hash devolve o hash do servidor como está no arquivo.
	Hash(name string) (string, bool)
	// HashOf devolve o hash que o servidor teria depois de Set(name, v).
	HashOf(name string, v map[string]any) (string, error)
	Set(name string, v map[string]any) error
	Delete(name string)
	Bytes() []byte
}

// Parse abre raw (nil ou vazio = arquivo novo) no formato pedido.
func Parse(f Format, raw []byte, section string) (Doc, error) {
	switch f {
	case JSON:
		return parseJSON(raw, section)
	case TOML:
		return parseTOML(raw, section)
	}
	return nil, fmt.Errorf("formato de configuração desconhecido %q", f)
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
