package names

import (
	"fmt"
	"os"
)

func LoadMappingFile(path, sourceCode, targetCode string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read names mapping file %s: %w", path, err)
	}
	mappings, err := DecodeMappings(data, sourceCode, targetCode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse names mapping file %s: %w", path, err)
	}
	nameDict := make(map[string]string)
	for _, m := range mappings {
		nameDict[m.Source] = m.Target
	}
	return nameDict, nil
}
