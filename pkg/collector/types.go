package collector

type energyMixDataFields struct {
	BioFuel                    float64 `json:"biofuel_TWh"`
	CarbonIntensity            float64 `json:"carbon_intensity"`
	Coal                       float64 `json:"coal_TWh"`
	CountryName                string  `json:"country_name"`
	Fossil                     float64 `json:"fossil_TWh"`
	Gas                        float64 `json:"gas_TWh"`
	Hydro                      float64 `json:"hydroelectricity_TWh"`
	IsoCode                    string  `json:"iso_code"`
	LowCarbon                  float64 `json:"low_carbon_TWh"`
	Nuclear                    float64 `json:"nuclear_TWh"`
	Oil                        float64 `json:"oil_TWh"`
	OtherRenewable             float64 `json:"other_renewable_TWh"`
	OtherRenewableExcluBioFuel float64 `json:"other_renewable_exc_biofuel_TWh"`
	PerCapita                  float64 `json:"per_capita_Wh"`
	Renewables                 float64 `json:"renewables_TWh"`
	Solar                      float64 `json:"solar_TWh"`
	Total                      float64 `json:"total_TWh"`
	Wind                       float64 `json:"wind_TWh"`
	Year                       int64   `json:"year"`
}
