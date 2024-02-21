package emissions

import (
	"net/http"

	"github.com/go-kit/log"
)

//nolint:misspell
// Opendatasoft API v1
// // Nicked from https://github.com/nmasse-itix/ego2mix
// type nationalRealTimeFields struct {
// 	Bioenergies              int64  `json:"bioenergies"`                 // Bioenergy (MW)
// 	BioenergiesBiogaz        int64  `json:"bioenergies_biogaz"`          // Bioenergy - Biogas (MW)
// 	BioenergiesBiomasse      int64  `json:"bioenergies_biomasse"`        // Bioenergy - Biomass (MW)
// 	BioenergiesDechets       int64  `json:"bioenergies_dechets"`         // Bioenergy - Waste (MW)
// 	Charbon                  int64  `json:"charbon"`                     // Carbon (MW)
// 	Consommation             int64  `json:"consommation"`                // Consumption (MW)
// 	Date                     string `json:"date"`                        // Date
// 	DateHeure                string `json:"date_heure"`                  // Date - Hour
// 	DestockageBatterie       string `json:"destockage_batterie"`         // Battery disposal (MW)
// 	EchCommAllemagneBelgique string `json:"ech_comm_allemagne_belgique"` // Exchange with Germany-Belgium (MW) - Commercial exchange - Exporter if negative, importer if positive.
// 	EchCommAngleterre        string `json:"ech_comm_angleterre"`         // Exchange with UK (MW)
// 	EchCommEspagne           int64  `json:"ech_comm_espagne"`            // Exchange with Spain (MW)
// 	EchCommItalie            int64  `json:"ech_comm_italie"`             // Exchange with Italy (MW)
// 	EchCommSuisse            int64  `json:"ech_comm_suisse"`             // Exchange with Swiss (MW)
// 	EchPhysiques             int64  `json:"ech_physiques"`               // Physical Exchange (MW)
// 	Eolien                   int64  `json:"eolien"`                      // Wind (MW)
// 	EolienOffshore           string `json:"eolien_offshore"`             // Wind offshore (MW)
// 	EolienTerrestre          string `json:"eolien_terrestre"`            // Wind terrain (MW)
// 	Fioul                    int64  `json:"fioul"`                       // Fuel (MW)
// 	FioulAutres              int64  `json:"fioul_autres"`                // Fuel - Other (MW)
// 	FioulCogen               int64  `json:"fioul_cogen"`                 // Fuel - Cogeneration (MW)
// 	FioulTac                 int64  `json:"fioul_tac"`                   // Fuel - Combustion turbines (MW)
// 	Gaz                      int64  `json:"gaz"`                         // Gas (MW)
// 	GazAutres                int64  `json:"gaz_autres"`                  // Gas - Others (MW)
// 	GazCcg                   int64  `json:"gaz_ccg"`                     // Gas - Gas Combined Cycles (MW)
// 	GazCogen                 int64  `json:"gaz_cogen"`                   // Gas - Cogeneration (MW)
// 	GazTac                   int64  `json:"gaz_tac"`                     // Gas - Combustion turbines (MW)
// 	Heure                    string `json:"heure"`                       // Hour
// 	Hydraulique              int64  `json:"hydraulique"`                 // Hydraulic (MW)
// 	HydrauliqueFilEauEclusee int64  `json:"hydraulique_fil_eau_eclusee"` // Hydraulic - River (MW)
// 	HydrauliqueLacs          int64  `json:"hydraulique_lacs"`            // Hydraulic - Lakes (MW)
// 	HydrauliqueStepTurbinage int64  `json:"hydraulique_step_turbinage"`  // Hydraulic - STEP turbine (MW)
// 	Nature                   string `json:"nature"`                      // Nature of data - Realtime.
// 	Nucleaire                int64  `json:"nucleaire"`                   // Nuclear (MW)
// 	Perimeter                string `json:"perimetre"`                   // nolint:misspell// Perimeter - France.
// 	Pompage                  int64  `json:"pompage"`                     // Pumping (MW) - Consumption by pumps at trasfer stations.
// 	PrevisionJ               int64  `json:"prevision_j"`                 // Forcaset J (MW) - For the same day.
// 	PrevisionJ1              int64  `json:"prevision_j1"`                // Forcaset J-1 (MW) - Forecast, made the day before for the following day, of consumption.
// 	Solaire                  int64  `json:"solaire"`                     // Solar (MW)
// 	StockageBatterie         string `json:"stockage_batterie"`           // Battery disposal (MW)
// 	TauxCo2                  int64  `json:"taux_co2"`                    // Emission factor (g/kWh)
// }

// type nationalRealTimeRecord struct {
// 	Datasetid       string                 `json:"datasetid"`
// 	Fields          nationalRealTimeFields `json:"fields"`
// 	RecordTimestamp string                 `json:"record_timestamp"`
// 	Recordid        string                 `json:"recordid"`
// }

// // NationalRealTimeResponse represents the response to the "eco2mix-national-tr"
// // dataset from RTE.
// //
// // Documentations is available at:
// // https://odre.opendatasoft.com/explore/dataset/eco2mix-national-tr/information/
// type nationalRealTimeResponse struct {
// 	FacetGroups []struct {
// 		Facets []struct {
// 			Count int64  `json:"count"`
// 			Name  string `json:"name"`
// 			Path  string `json:"path"`
// 			State string `json:"state"`
// 		} `json:"facets"`
// 		Name string `json:"name"`
// 	} `json:"facet_groups"`
// 	Nhits      int64 `json:"nhits"`
// 	Parameters struct {
// 		Dataset  string   `json:"dataset"`
// 		Facet    []string `json:"facet"`
// 		Format   string   `json:"format"`
// 		Rows     int64    `json:"rows"`
// 		Start    int64    `json:"start"`
// 		Timezone string   `json:"timezone"`
// 	} `json:"parameters"`
// 	Records []nationalRealTimeRecord `json:"records"`
// }

// Opendatasoft API v2
// Ref: https://reseaux-energies-rte.opendatasoft.com/api/explore/v2.1/console
// Ref: https://help.opendatasoft.com/apis/ods-explore-v2/
type nationalRealTimeFieldsV2 struct {
	TauxCo2   int64  `json:"taux_co2"`
	DateHeure string `json:"date_heure"`
}

type nationalRealTimeResponseV2 struct {
	TotalCount int                        `json:"total_count"`
	Results    []nationalRealTimeFieldsV2 `json:"results"`
}

// code carbon global energy data mix interface
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

// CountryCodeFields contains different ISO codes of a given country
type CountryCodeFields struct {
	Alpha2Code    string `json:"alpha_2"`
	Alpha3Code    string `json:"alpha_3"`
	Name          string `json:"name"`
	NumericalCode string `json:"numeric"`
}

// CountryCode contains data of countries ISO codes
type CountryCode struct {
	IsoCode []CountryCodeFields `json:"3166-1"`
}

// Electricity Maps response signature
type eMapsResponse struct {
	Zone               string `json:"zone"`
	CarbonIntensity    int    `json:"carbonIntensity"`
	DateTime           string `json:"datetime"`
	UpdatedAt          string `json:"updatedAt"`
	EmissionFactorType string `json:"emissionFactorType"`
	IsEstimated        bool   `json:"isEstimated"`
	EstimationMethod   string `json:"estimationMethod"`
}

// ContextKey is the struct key to set values in context
type ContextKey struct{}

// ContextValues contains the values to be set in context
type ContextValues struct {
	CountryCodeAlpha2 string
	CountryCodeAlpha3 string
}

// Client interface
type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// Provider is the interface a emission provider has to implement.
type Provider interface {
	// Update current emission factor
	Update() (float64, error)
}

// FactorProviders implements the interface to collect
// emission factors from different sources.
type FactorProviders struct {
	Providers     map[string]Provider
	ProviderNames map[string]string
	logger        log.Logger
}

// PayLoad contains emissions factor
type PayLoad struct {
	Factor float64
	Name   string
}
