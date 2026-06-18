package postpolish

import (
	"fmt"
	"strings"
)

func segmentInstructionSchema(segments []RequestSegment) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"segments"},
		"properties": map[string]any{
			"segments": exactSegmentArraySchema(segments, []string{"id", "source_text", "edit_instruction"}, func(segment RequestSegment) map[string]any {
				return map[string]any{
					"id": map[string]any{
						"type":  "integer",
						"const": segment.ID,
					},
					"source_text": map[string]any{
						"type":  "string",
						"const": segment.SourceText,
					},
					"edit_instruction": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
				}
			}),
		},
	}
}

func chunkInstructionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"edit_instruction"},
		"properties": map[string]any{
			"edit_instruction": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}

func applicationSchema(segments []RequestSegment) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"segments"},
		"properties": map[string]any{
			"segments": exactSegmentArraySchema(segments, []string{"id", "source_text", "text"}, func(segment RequestSegment) map[string]any {
				return map[string]any{
					"id": map[string]any{
						"type":  "integer",
						"const": segment.ID,
					},
					"source_text": map[string]any{
						"type":  "string",
						"const": segment.SourceText,
					},
					"text": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
				}
			}),
		},
	}
}

func exactSegmentArraySchema(segments []RequestSegment, required []string, properties func(RequestSegment) map[string]any) map[string]any {
	prefixItems := make([]any, 0, len(segments))
	for _, segment := range segments {
		prefixItems = append(prefixItems, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties":           properties(segment),
		})
	}
	return map[string]any{
		"type":        "array",
		"minItems":    len(segments),
		"maxItems":    len(segments),
		"prefixItems": prefixItems,
	}
}

func validateSegmentInstructions(chunk v2Chunk, rows []SegmentInstruction) ([]SegmentInstruction, error) {
	if len(rows) != len(chunk.Segments) {
		return nil, fmt.Errorf("segment instruction count mismatch: got %d want %d", len(rows), len(chunk.Segments))
	}
	out := make([]SegmentInstruction, 0, len(rows))
	for index, input := range chunk.Segments {
		row := rows[index]
		if row.ID != input.ID {
			return nil, fmt.Errorf("instruction id mismatch at row %d: got %d want %d", index, row.ID, input.ID)
		}
		if row.SourceText != input.SourceText {
			return nil, fmt.Errorf("instruction source_text mismatch at row %d", index)
		}
		instruction := strings.TrimSpace(row.EditInstruction)
		if instruction == "" {
			instruction = "No change needed."
		}
		out = append(out, SegmentInstruction{
			ID:              row.ID,
			SourceText:      row.SourceText,
			EditInstruction: instruction,
		})
	}
	return out, nil
}
