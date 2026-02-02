package regresql

import (
	"fmt"
	"math/rand"
)

// ISO 3166-1 alpha-2 country codes (most common 100)
var countryCodes = []string{
	"US", "GB", "DE", "FR", "CA", "AU", "JP", "BR", "IN", "MX",
	"ES", "IT", "NL", "SE", "CH", "AT", "BE", "PL", "PT", "DK",
	"NO", "FI", "IE", "NZ", "SG", "HK", "KR", "TW", "IL", "ZA",
	"AR", "CL", "CO", "PE", "CZ", "HU", "RO", "GR", "TR", "TH",
	"MY", "PH", "ID", "VN", "AE", "SA", "EG", "NG", "KE", "UA",
	"RU", "CN", "PK", "BD", "ET", "CD", "TZ", "MM", "KH", "NP",
	"LK", "GH", "UG", "DZ", "MA", "TN", "LY", "SD", "IQ", "AF",
	"IR", "SY", "JO", "LB", "KW", "QA", "BH", "OM", "YE", "PS",
	"HR", "RS", "BG", "SK", "SI", "LT", "LV", "EE", "MT", "CY",
	"IS", "LU", "MC", "AD", "SM", "LI", "VA", "BA", "MK", "AL",
}

// CountryCodeGenerator generates ISO country codes
type CountryCodeGenerator struct {
	BaseGenerator
}

func NewCountryCodeGenerator() *CountryCodeGenerator {
	return &CountryCodeGenerator{
		BaseGenerator: BaseGenerator{name: "country_code"},
	}
}

func (g *CountryCodeGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	mode := getParam(params, "mode", "cycle")

	switch mode {
	case "cycle":
		// Use _index to cycle through codes (for unique values)
		index := getParam(params, "_index", 0)
		return countryCodes[index%len(countryCodes)], nil

	case "random":
		// Pick random country code
		return countryCodes[rand.Intn(len(countryCodes))], nil

	default:
		return nil, fmt.Errorf("country_code: unknown mode %q (use 'cycle' or 'random')", mode)
	}
}

func (g *CountryCodeGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	mode := getParam(params, "mode", "cycle")
	if mode != "cycle" && mode != "random" {
		return fmt.Errorf("country_code: mode must be 'cycle' or 'random'")
	}
	return nil
}
