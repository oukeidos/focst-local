package names

import (
	"encoding/json"
	"fmt"

	"github.com/oukeidos/focst-local/internal/language"
)

func normalizeCode(code string) (string, error) {
	lang, ok := language.GetLanguage(code)
	if !ok {
		return "", fmt.Errorf("unsupported language: %s", code)
	}
	return lang.Code, nil
}

func schemaKeys(sourceCode, targetCode string) (string, string, error) {
	src, err := normalizeCode(sourceCode)
	if err != nil {
		return "", "", err
	}
	tgt, err := normalizeCode(targetCode)
	if err != nil {
		return "", "", err
	}
	return src, tgt, nil
}

func EncodeMappings(mappings []CharacterMapping, sourceCode, targetCode string) ([]byte, error) {
	sourceKey, targetKey, err := schemaKeys(sourceCode, targetCode)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, map[string]string{
			sourceKey: m.Source,
			targetKey: m.Target,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}

func DecodeMappings(data []byte, sourceCode, targetCode string) ([]CharacterMapping, error) {
	sourceKey, targetKey, err := schemaKeys(sourceCode, targetCode)
	if err != nil {
		return nil, err
	}
	var raw []map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	mappings := make([]CharacterMapping, 0, len(raw))
	for _, entry := range raw {
		srcVal, ok := entry[sourceKey]
		if !ok {
			return nil, fmt.Errorf("missing source field %q", sourceKey)
		}
		tgtVal, ok := entry[targetKey]
		if !ok {
			return nil, fmt.Errorf("missing target field %q", targetKey)
		}
		mappings = append(mappings, CharacterMapping{
			Source: srcVal,
			Target: tgtVal,
		})
	}
	return mappings, nil
}
