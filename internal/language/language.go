package language

import (
	"sort"
)

// Language represents a supported language with its configuration.
type Language struct {
	Code       string
	Name       string
	DefaultCPS int // Characters Per Second
}

// Default settings as requested
const (
	DefaultCPS = 17
)

// Languages is a map of supported languages code -> Language.
var Languages = map[string]Language{
	"af":       {Code: "af", Name: "Afrikaans", DefaultCPS: DefaultCPS},
	"sq":       {Code: "sq", Name: "Albanian", DefaultCPS: DefaultCPS}, // fallback
	"am":       {Code: "am", Name: "Amharic", DefaultCPS: DefaultCPS},  // fallback
	"ar":       {Code: "ar", Name: "Arabic", DefaultCPS: 20},
	"hy":       {Code: "hy", Name: "Armenian", DefaultCPS: DefaultCPS},    // fallback
	"as":       {Code: "as", Name: "Assamese", DefaultCPS: DefaultCPS},    // fallback
	"az":       {Code: "az", Name: "Azerbaijani", DefaultCPS: DefaultCPS}, // fallback
	"eu":       {Code: "eu", Name: "Basque", DefaultCPS: DefaultCPS},
	"be":       {Code: "be", Name: "Belarusian", DefaultCPS: DefaultCPS}, // fallback
	"bn":       {Code: "bn", Name: "Bengali", DefaultCPS: 22},
	"bs":       {Code: "bs", Name: "Bosnian", DefaultCPS: DefaultCPS}, // fallback
	"bg":       {Code: "bg", Name: "Bulgarian", DefaultCPS: DefaultCPS},
	"ca":       {Code: "ca", Name: "Catalan", DefaultCPS: DefaultCPS},
	"ceb":      {Code: "ceb", Name: "Cebuano", DefaultCPS: DefaultCPS},          // fallback
	"zh":       {Code: "zh-Hans", Name: "Chinese (Simplified)", DefaultCPS: 11}, // Default to Simplified
	"zh-Hans":  {Code: "zh-Hans", Name: "Chinese (Simplified)", DefaultCPS: 11},
	"zh-Hant":  {Code: "zh-Hant", Name: "Chinese (Traditional)", DefaultCPS: 11},
	"co":       {Code: "co", Name: "Corsican", DefaultCPS: DefaultCPS}, // fallback
	"hr":       {Code: "hr", Name: "Croatian", DefaultCPS: DefaultCPS},
	"cs":       {Code: "cs", Name: "Czech", DefaultCPS: DefaultCPS},
	"da":       {Code: "da", Name: "Danish", DefaultCPS: DefaultCPS},
	"dv":       {Code: "dv", Name: "Dhivehi", DefaultCPS: DefaultCPS}, // fallback
	"nl":       {Code: "nl", Name: "Dutch", DefaultCPS: DefaultCPS},
	"en":       {Code: "en", Name: "English", DefaultCPS: 20},
	"eo":       {Code: "eo", Name: "Esperanto", DefaultCPS: DefaultCPS}, // fallback
	"et":       {Code: "et", Name: "Estonian", DefaultCPS: DefaultCPS},  // fallback
	"fil":      {Code: "fil", Name: "Filipino", DefaultCPS: DefaultCPS},
	"fi":       {Code: "fi", Name: "Finnish", DefaultCPS: DefaultCPS},
	"fr":       {Code: "fr", Name: "French", DefaultCPS: DefaultCPS},
	"fy":       {Code: "fy", Name: "Frisian", DefaultCPS: DefaultCPS}, // fallback
	"gl":       {Code: "gl", Name: "Galician", DefaultCPS: DefaultCPS},
	"ka":       {Code: "ka", Name: "Georgian", DefaultCPS: DefaultCPS}, // fallback
	"de":       {Code: "de", Name: "German", DefaultCPS: DefaultCPS},
	"el":       {Code: "el", Name: "Greek", DefaultCPS: DefaultCPS},
	"gu":       {Code: "gu", Name: "Gujarati", DefaultCPS: DefaultCPS},       // fallback
	"ht":       {Code: "ht", Name: "Haitian Creole", DefaultCPS: DefaultCPS}, // fallback
	"ha":       {Code: "ha", Name: "Hausa", DefaultCPS: DefaultCPS},          // fallback
	"haw":      {Code: "haw", Name: "Hawaiian", DefaultCPS: DefaultCPS},      // fallback
	"iw":       {Code: "iw", Name: "Hebrew", DefaultCPS: DefaultCPS},
	"hi":       {Code: "hi", Name: "Hindi", DefaultCPS: 22},
	"hmn":      {Code: "hmn", Name: "Hmong", DefaultCPS: DefaultCPS}, // fallback
	"hu":       {Code: "hu", Name: "Hungarian", DefaultCPS: DefaultCPS},
	"is":       {Code: "is", Name: "Icelandic", DefaultCPS: DefaultCPS},
	"ig":       {Code: "ig", Name: "Igbo", DefaultCPS: DefaultCPS}, // fallback
	"id":       {Code: "id", Name: "Indonesian", DefaultCPS: DefaultCPS},
	"ga":       {Code: "ga", Name: "Irish", DefaultCPS: DefaultCPS},
	"it":       {Code: "it", Name: "Italian", DefaultCPS: DefaultCPS},
	"ja":       {Code: "ja", Name: "Japanese", DefaultCPS: 4},          // fallback (CPS)
	"jv":       {Code: "jv", Name: "Javanese", DefaultCPS: DefaultCPS}, // fallback
	"kn":       {Code: "kn", Name: "Kannada", DefaultCPS: 22},
	"kk":       {Code: "kk", Name: "Kazakh", DefaultCPS: DefaultCPS}, // fallback
	"km":       {Code: "km", Name: "Khmer", DefaultCPS: DefaultCPS},  // fallback
	"ko":       {Code: "ko", Name: "Korean", DefaultCPS: 12},
	"kri":      {Code: "kri", Name: "Krio", DefaultCPS: DefaultCPS},         // fallback
	"ku":       {Code: "ku", Name: "Kurdish", DefaultCPS: DefaultCPS},       // fallback
	"ky":       {Code: "ky", Name: "Kyrgyz", DefaultCPS: DefaultCPS},        // fallback
	"lo":       {Code: "lo", Name: "Lao", DefaultCPS: DefaultCPS},           // fallback
	"la":       {Code: "la", Name: "Latin", DefaultCPS: DefaultCPS},         // fallback
	"lv":       {Code: "lv", Name: "Latvian", DefaultCPS: DefaultCPS},       // fallback
	"lt":       {Code: "lt", Name: "Lithuanian", DefaultCPS: DefaultCPS},    // fallback
	"lb":       {Code: "lb", Name: "Luxembourgish", DefaultCPS: DefaultCPS}, // fallback
	"mk":       {Code: "mk", Name: "Macedonian", DefaultCPS: DefaultCPS},    // fallback
	"mg":       {Code: "mg", Name: "Malagasy", DefaultCPS: DefaultCPS},      // fallback
	"ms":       {Code: "ms", Name: "Malay", DefaultCPS: DefaultCPS},
	"ml":       {Code: "ml", Name: "Malayalam", DefaultCPS: 22},
	"mt":       {Code: "mt", Name: "Maltese", DefaultCPS: DefaultCPS}, // fallback
	"mi":       {Code: "mi", Name: "Maori", DefaultCPS: DefaultCPS},   // fallback
	"mr":       {Code: "mr", Name: "Marathi", DefaultCPS: 22},
	"mni-Mtei": {Code: "mni-Mtei", Name: "Meiteilon (Manipuri)", DefaultCPS: DefaultCPS}, // fallback
	"mn":       {Code: "mn", Name: "Mongolian", DefaultCPS: DefaultCPS},                  // fallback
	"my":       {Code: "my", Name: "Myanmar (Burmese)", DefaultCPS: DefaultCPS},          // fallback
	"ne":       {Code: "ne", Name: "Nepali", DefaultCPS: DefaultCPS},                     // fallback
	"no":       {Code: "no", Name: "Norwegian", DefaultCPS: DefaultCPS},
	"ny":       {Code: "ny", Name: "Nyanja (Chichewa)", DefaultCPS: DefaultCPS}, // fallback
	"or":       {Code: "or", Name: "Odia (Oriya)", DefaultCPS: DefaultCPS},      // fallback
	"ps":       {Code: "ps", Name: "Pashto", DefaultCPS: DefaultCPS},            // fallback
	"fa":       {Code: "fa", Name: "Persian", DefaultCPS: DefaultCPS},           // fallback
	"pl":       {Code: "pl", Name: "Polish", DefaultCPS: DefaultCPS},
	"pt":       {Code: "pt", Name: "Portuguese", DefaultCPS: DefaultCPS},
	"pa":       {Code: "pa", Name: "Punjabi", DefaultCPS: DefaultCPS}, // fallback
	"ro":       {Code: "ro", Name: "Romanian", DefaultCPS: DefaultCPS},
	"ru":       {Code: "ru", Name: "Russian", DefaultCPS: DefaultCPS},
	"sm":       {Code: "sm", Name: "Samoan", DefaultCPS: DefaultCPS},       // fallback
	"gd":       {Code: "gd", Name: "Scots Gaelic", DefaultCPS: DefaultCPS}, // fallback
	"sr":       {Code: "sr", Name: "Serbian", DefaultCPS: DefaultCPS},
	"st":       {Code: "st", Name: "Sesotho", DefaultCPS: DefaultCPS},             // fallback
	"sn":       {Code: "sn", Name: "Shona", DefaultCPS: DefaultCPS},               // fallback
	"sd":       {Code: "sd", Name: "Sindhi", DefaultCPS: DefaultCPS},              // fallback
	"si":       {Code: "si", Name: "Sinhala (Sinhalese)", DefaultCPS: DefaultCPS}, // fallback
	"sk":       {Code: "sk", Name: "Slovak", DefaultCPS: DefaultCPS},
	"sl":       {Code: "sl", Name: "Slovenian", DefaultCPS: DefaultCPS}, // fallback
	"so":       {Code: "so", Name: "Somali", DefaultCPS: DefaultCPS},    // fallback
	"es":       {Code: "es", Name: "Spanish", DefaultCPS: DefaultCPS},
	"su":       {Code: "su", Name: "Sundanese", DefaultCPS: DefaultCPS}, // fallback
	"sw":       {Code: "sw", Name: "Swahili", DefaultCPS: DefaultCPS},   // fallback
	"sv":       {Code: "sv", Name: "Swedish", DefaultCPS: DefaultCPS},
	"tg":       {Code: "tg", Name: "Tajik", DefaultCPS: DefaultCPS}, // fallback
	"ta":       {Code: "ta", Name: "Tamil", DefaultCPS: 22},
	"te":       {Code: "te", Name: "Telugu", DefaultCPS: DefaultCPS},
	"th":       {Code: "th", Name: "Thai", DefaultCPS: DefaultCPS},
	"tr":       {Code: "tr", Name: "Turkish", DefaultCPS: DefaultCPS},
	"uk":       {Code: "uk", Name: "Ukrainian", DefaultCPS: DefaultCPS},
	"ur":       {Code: "ur", Name: "Urdu", DefaultCPS: DefaultCPS},   // fallback
	"ug":       {Code: "ug", Name: "Uyghur", DefaultCPS: DefaultCPS}, // fallback
	"uz":       {Code: "uz", Name: "Uzbek", DefaultCPS: DefaultCPS},  // fallback
	"vi":       {Code: "vi", Name: "Vietnamese", DefaultCPS: DefaultCPS},
	"cy":       {Code: "cy", Name: "Welsh", DefaultCPS: DefaultCPS},
	"xh":       {Code: "xh", Name: "Xhosa", DefaultCPS: DefaultCPS},   // fallback
	"yi":       {Code: "yi", Name: "Yiddish", DefaultCPS: DefaultCPS}, // fallback
	"yo":       {Code: "yo", Name: "Yoruba", DefaultCPS: DefaultCPS},  // fallback
	"zu":       {Code: "zu", Name: "Zulu", DefaultCPS: DefaultCPS},
}

// GetLanguageCode returns strict matching code or empty if not found.
func GetLanguage(code string) (Language, bool) {
	lang, ok := Languages[code]
	return lang, ok
}

// LanguageEntry represents a map entry for listing.
type LanguageEntry struct {
	ID string // The map key (CLI flag)
	Language
}

// GetSupportedLanguages returns a list of supported languages sorted by Name and then ID.
func GetSupportedLanguages() []LanguageEntry {
	entries := make([]LanguageEntry, 0, len(Languages))
	for k, v := range Languages {
		entries = append(entries, LanguageEntry{ID: k, Language: v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].ID < entries[j].ID
	})
	return entries
}
