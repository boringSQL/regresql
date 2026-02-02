package regresql

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// HistogramGenerator generates values based on histogram bucket distribution
type HistogramGenerator struct {
	BaseGenerator
}

func NewHistogramGenerator() *HistogramGenerator {
	return &HistogramGenerator{
		BaseGenerator: BaseGenerator{name: "histogram"},
	}
}

func (g *HistogramGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	nullProb := getParam(params, "null_probability", 0.0)

	// Handle null probability
	if nullProb > 0 && rand.Float64() < nullProb {
		return nil, nil
	}

	boundsRaw, ok := params["bounds"]
	if !ok {
		return nil, fmt.Errorf("histogram generator requires 'bounds' parameter")
	}

	bounds, err := parseBounds(boundsRaw)
	if err != nil {
		return nil, err
	}

	if len(bounds) < 2 {
		return nil, fmt.Errorf("histogram generator requires at least 2 bounds")
	}

	// Detect value type from bounds
	valueType := detectBoundsType(bounds)

	// Pick a random bucket (equal probability)
	bucketCount := len(bounds) - 1
	bucket := rand.Intn(bucketCount)

	// Generate value within bucket
	return generateInBucket(bounds[bucket], bounds[bucket+1], valueType)
}

func (g *HistogramGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	boundsRaw, ok := params["bounds"]
	if !ok {
		return fmt.Errorf("histogram generator requires 'bounds' parameter")
	}

	bounds, err := parseBounds(boundsRaw)
	if err != nil {
		return err
	}

	if len(bounds) < 2 {
		return fmt.Errorf("histogram generator requires at least 2 bounds")
	}

	nullProb := getParam(params, "null_probability", 0.0)
	if nullProb < 0 || nullProb > 1 {
		return fmt.Errorf("null_probability must be between 0 and 1")
	}

	return nil
}

type boundsType int

const (
	boundsTypeString boundsType = iota
	boundsTypeInt
	boundsTypeFloat
	boundsTypeTimestamp
	boundsTypeDate
)

func parseBounds(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			switch val := item.(type) {
			case string:
				result[i] = val
			case int:
				result[i] = strconv.Itoa(val)
			case int64:
				result[i] = strconv.FormatInt(val, 10)
			case float64:
				result[i] = strconv.FormatFloat(val, 'f', -1, 64)
			default:
				result[i] = fmt.Sprintf("%v", val)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("bounds must be an array, got %T", raw)
	}
}

func detectBoundsType(bounds []string) boundsType {
	if len(bounds) == 0 {
		return boundsTypeString
	}

	sample := bounds[0]

	// Try parsing as int
	if _, err := strconv.ParseInt(sample, 10, 64); err == nil {
		return boundsTypeInt
	}

	// Try parsing as float
	if _, err := strconv.ParseFloat(sample, 64); err == nil {
		return boundsTypeFloat
	}

	// Try parsing as timestamp (various formats)
	timestampFormats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range timestampFormats {
		if _, err := time.Parse(format, sample); err == nil {
			if format == "2006-01-02" {
				return boundsTypeDate
			}
			return boundsTypeTimestamp
		}
	}

	return boundsTypeString
}

func generateInBucket(low, high string, vtype boundsType) (any, error) {
	switch vtype {
	case boundsTypeInt:
		lowInt, _ := strconv.ParseInt(low, 10, 64)
		highInt, _ := strconv.ParseInt(high, 10, 64)
		if highInt <= lowInt {
			return lowInt, nil
		}
		return lowInt + rand.Int63n(highInt-lowInt), nil

	case boundsTypeFloat:
		lowFloat, _ := strconv.ParseFloat(low, 64)
		highFloat, _ := strconv.ParseFloat(high, 64)
		if highFloat <= lowFloat {
			return lowFloat, nil
		}
		return lowFloat + rand.Float64()*(highFloat-lowFloat), nil

	case boundsTypeTimestamp:
		lowTime := parseTimestamp(low)
		highTime := parseTimestamp(high)
		if highTime.Before(lowTime) || highTime.Equal(lowTime) {
			return lowTime, nil
		}
		diff := highTime.Unix() - lowTime.Unix()
		randomSeconds := rand.Int63n(diff)
		return lowTime.Add(time.Duration(randomSeconds) * time.Second), nil

	case boundsTypeDate:
		lowDate := parseDate(low)
		highDate := parseDate(high)
		if highDate.Before(lowDate) || highDate.Equal(lowDate) {
			return lowDate, nil
		}
		diff := highDate.Unix() - lowDate.Unix()
		randomSeconds := rand.Int63n(diff)
		return lowDate.Add(time.Duration(randomSeconds) * time.Second), nil

	default:
		// For strings, just return the low bound
		// (string histograms don't make much sense for generation)
		return low, nil
	}
}

func parseTimestamp(s string) time.Time {
	// Clean up PostgreSQL-style timestamp
	s = strings.TrimSpace(s)

	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}

	// Fallback to now
	return time.Now()
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t
	}
	return time.Now()
}
