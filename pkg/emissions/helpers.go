package emissions

import "encoding/json"

var CountryCodes CountryCode

func init() {
	// Read countries JSON file
	countryCodesContents, err := dataDir.ReadFile("data/data_iso_3166-1.json")
	if err != nil {
		return
	}

	// Unmarshal JSON file into struct
	if err := json.Unmarshal(countryCodesContents, &CountryCodes); err != nil {
		return
	}
}

// ISO32Map returns a ISO-3 code to ISO-2 code map.
func ISO32Map() map[string]string {
	codeMap := make(map[string]string)
	for _, country := range CountryCodes.IsoCode {
		codeMap[country.Alpha3Code] = country.Alpha2Code
	}

	return codeMap
}

// ISO23Map returns a ISO-2 code to ISO-3 code map.
func ISO23Map() map[string]string {
	codeMap := make(map[string]string)
	for _, country := range CountryCodes.IsoCode {
		codeMap[country.Alpha2Code] = country.Alpha3Code
	}

	return codeMap
}
