package pipeline

import "github.com/oukeidos/focst-local/internal/translation"

// TranslationStatus is the terminal state of a translation run.
type TranslationStatus string

const (
	TranslationStatusSuccess        TranslationStatus = "Success"
	TranslationStatusPartialSuccess TranslationStatus = "Partial Success"
	TranslationStatusFailure        TranslationStatus = "Failure"
	TranslationStatusSkipped        TranslationStatus = "Skipped"
)

// TranslationResult contains structured outputs from RunTranslation.
type TranslationResult struct {
	Status          TranslationStatus
	RecoveryLogPath string
	OutputPath      string
	Usage           translation.UsageMetadata
	FailedChunks    int
	TotalChunks     int
}

func translationStatusFromRecovery(status string) TranslationStatus {
	switch status {
	case string(TranslationStatusSuccess):
		return TranslationStatusSuccess
	case string(TranslationStatusPartialSuccess):
		return TranslationStatusPartialSuccess
	case string(TranslationStatusFailure):
		return TranslationStatusFailure
	default:
		return TranslationStatusFailure
	}
}
