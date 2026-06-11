package licenses

import _ "embed"

//go:embed embedded/LICENSE
var licenseText string

//go:embed embedded/THIRD_PARTY_NOTICES.md
var noticesText string

//go:embed embedded/THIRD_PARTY_LICENSES_FULL.txt
var fullText string

//go:embed embedded/DISCLAIMER.md
var disclaimerText string

func LicenseText() string {
	return licenseText
}

func NoticesText() string {
	return noticesText
}

func FullText() string {
	return fullText
}

func DisclaimerText() string {
	return disclaimerText
}
