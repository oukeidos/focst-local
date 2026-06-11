package metadata

type OpenAIModel struct {
	ID               string
	Label            string
	InputPerMillion  float64
	OutputPerMillion float64
}

var OpenAIModels = []OpenAIModel{
	{
		ID:               "gpt-5.2",
		Label:            "GPT-5.2",
		InputPerMillion:  1.75,
		OutputPerMillion: 14.00,
	},
}

const (
	DefaultOpenAIInputPerMillion  = 2.50
	DefaultOpenAIOutputPerMillion = 10.00
	WebSearchCostPerCall          = 0.01
)

func OpenAIPricing(modelID string) (OpenAIModel, bool) {
	for _, m := range OpenAIModels {
		if m.ID == modelID {
			return m, true
		}
	}
	return OpenAIModel{
		ID:               "default",
		Label:            "Default OpenAI",
		InputPerMillion:  DefaultOpenAIInputPerMillion,
		OutputPerMillion: DefaultOpenAIOutputPerMillion,
	}, false
}
