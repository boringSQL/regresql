package regresql

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

const defaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var (
	emailPrefixes = []string{"user", "admin", "test", "info", "contact", "support", "hello"}
	emailDomains  = []string{"example.com", "sql.test", "globex.test", "testify.example.org"}
	firstNames    = []string{
		"Aisha", "Amara", "Amelia", "Ananya", "Anastasia", "Antonio", "Arjun", "Astrid",
		"Ayanna", "Camille", "Carmen", "Carlos", "Chen", "Diego", "Dimitri", "Elena",
		"Emma", "Fatima", "Gabriela", "Giulia", "Greta", "Hans", "Hassan", "Henry",
		"Hiroshi", "Ingrid", "Isabella", "Jabari", "James", "Jean", "Katarina", "Khalid",
		"Kim", "Kofi", "Kwame", "Lars", "Layla", "Lee", "Lin", "Luca", "Lucia",
		"Marco", "Marie", "Mateo", "Mei", "Miguel", "Ming", "Nguyen", "Nia", "Niklas",
		"Oliver", "Omar", "Park", "Pierre", "Piotr", "Priya", "Raj", "Sakura", "Santiago",
		"Sofia", "Sophia", "Thabo", "Valentina", "Viktor", "Wavey", "Wei", "William",
		"Yuki", "Yusuf", "Zara", "Zofia", "Zuri",
	}
	lastNames = []string{
		"Ahmed", "Ali", "Banda", "Becker", "Bernard", "Bianchi", "Brown", "Chen",
		"Choi", "Colombo", "Davies", "Diallo", "Dubois", "Dupont", "Esposito", "Evans",
		"Ferrari", "Fischer", "García", "González", "Gupta", "Hassan", "Huang", "Ibrahim",
		"Ito", "Ivanov", "Johnson", "Jung", "Kamiński", "Kang", "Khalil", "Khan",
		"Kim", "Kobayashi", "Kone", "Kowalczyk", "Kowalski", "Kumar", "Laurent", "Lebedev",
		"Lee", "Leroy", "Li", "Liu", "López", "Mahmoud", "Mansour", "Martin",
		"Martínez", "Mensah", "Meyer", "Mohamed", "Moreau", "Müller", "Mwangi", "Nakamura",
		"Nkosi", "Nowak", "Okafor", "Park", "Patel", "Pérez", "Petrov", "Popov",
		"Quoyle", "Ramírez", "Reddy", "Ricci", "Roberts", "Rodríguez", "Romano", "Rossi",
		"Russo", "Sánchez", "Schmidt", "Schneider", "Sharma", "Sidorov", "Simon", "Singh",
		"Smith", "Sokolov", "Suzuki", "Takahashi", "Tanaka", "Taylor", "Traore", "Verma",
		"Wagner", "Wang", "Watanabe", "Weber", "Wilson", "Wiśniewski", "Wojciechowski", "Yamamoto",
		"Yang", "Zhang", "Zhao",
	}
)

type (
	// SequenceGenerator generates sequential integers
	SequenceGenerator struct {
		BaseGenerator
		counter int64
	}

	// IntGenerator generates random integers
	IntGenerator struct {
		BaseGenerator
	}

	// StringGenerator generates random strings
	StringGenerator struct {
		BaseGenerator
	}

	// UUIDGenerator generates UUIDs
	UUIDGenerator struct {
		BaseGenerator
	}

	// EmailGenerator generates realistic email addresses
	EmailGenerator struct {
		BaseGenerator
	}

	// NameGenerator generates realistic names
	NameGenerator struct {
		BaseGenerator
	}

	// NowGenerator generates current timestamp
	NowGenerator struct {
		BaseGenerator
	}

	// DateBetweenGenerator generates random dates within a range
	DateBetweenGenerator struct {
		BaseGenerator
	}

	// DecimalGenerator generates random decimal numbers
	DecimalGenerator struct {
		BaseGenerator
	}
)

func NewSequenceGenerator() *SequenceGenerator {
	return &SequenceGenerator{
		BaseGenerator: BaseGenerator{name: "sequence"},
		counter:       0,
	}
}

func (g *SequenceGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	start := getParam(params, "start", int64(1))

	if g.counter == 0 {
		g.counter = start
	}

	value := g.counter
	g.counter++

	return value, nil
}

func (g *SequenceGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	return nil
}

func NewIntGenerator() *IntGenerator {
	return &IntGenerator{
		BaseGenerator: BaseGenerator{name: "int"},
	}
}

func (g *IntGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	min := getParam(params, "min", int64(0))
	max := getParam(params, "max", int64(1000000))

	if max <= min {
		return nil, fmt.Errorf("max must be greater than min")
	}

	value := min + rand.Int63n(max-min)
	return value, nil
}

func (g *IntGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	min := getParam(params, "min", int64(0))
	max := getParam(params, "max", int64(1000000))

	if max <= min {
		return fmt.Errorf("max (%d) must be greater than min (%d)", max, min)
	}

	return nil
}

func NewStringGenerator() *StringGenerator {
	return &StringGenerator{
		BaseGenerator: BaseGenerator{name: "string"},
	}
}

func (g *StringGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	length := getParam(params, "length", 10)
	charset := getParam(params, "charset", defaultCharset)

	if length <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}

	// Apply column max length constraint if available
	if column.MaxLength != nil && length > *column.MaxLength {
		length = *column.MaxLength
	}

	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}

	return string(result), nil
}

func (g *StringGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	length := getParam(params, "length", 10)

	if length <= 0 {
		return fmt.Errorf("length must be positive")
	}

	return nil
}

func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{
		BaseGenerator: BaseGenerator{name: "uuid"},
	}
}

func (g *UUIDGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	version := getParam(params, "version", "v4")

	switch version {
	case "v4":
		return uuid.New().String(), nil
	case "v7":
		return uuid.Must(uuid.NewV7()).String(), nil
	default:
		return nil, fmt.Errorf("unsupported UUID version: %s", version)
	}
}

func (g *UUIDGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	version := getParam(params, "version", "v4")

	switch version {
	case "v4", "v7":
		return nil
	default:
		return fmt.Errorf("unsupported UUID version: %s (must be v4 or v7)", version)
	}
}

func NewEmailGenerator() *EmailGenerator {
	return &EmailGenerator{
		BaseGenerator: BaseGenerator{name: "email"},
	}
}

func (g *EmailGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	domain := getParam(params, "domain", "")

	prefix := emailPrefixes[rand.Intn(len(emailPrefixes))]
	suffix := rand.Intn(10000)

	if domain == "" {
		domain = emailDomains[rand.Intn(len(emailDomains))]
	}

	email := fmt.Sprintf("%s%d@%s", prefix, suffix, domain)
	return email, nil
}

func (g *EmailGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	return nil
}

func NewNameGenerator() *NameGenerator {
	return &NameGenerator{
		BaseGenerator: BaseGenerator{name: "name"},
	}
}

func (g *NameGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	nameType := getParam(params, "type", "full")

	firstName := firstNames[rand.Intn(len(firstNames))]
	lastName := lastNames[rand.Intn(len(lastNames))]

	switch nameType {
	case "first":
		return firstName, nil
	case "last":
		return lastName, nil
	case "full":
		return fmt.Sprintf("%s %s", firstName, lastName), nil
	default:
		return nil, fmt.Errorf("unsupported name type: %s", nameType)
	}
}

func (g *NameGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	nameType := getParam(params, "type", "full")

	switch nameType {
	case "first", "last", "full":
		return nil
	default:
		return fmt.Errorf("unsupported name type: %s (must be first, last, or full)", nameType)
	}
}

func NewNowGenerator() *NowGenerator {
	return &NowGenerator{
		BaseGenerator: BaseGenerator{name: "now"},
	}
}

func (g *NowGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	return time.Now(), nil
}

func (g *NowGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	return nil
}

func NewDateBetweenGenerator() *DateBetweenGenerator {
	return &DateBetweenGenerator{
		BaseGenerator: BaseGenerator{name: "date_between"},
	}
}

func (g *DateBetweenGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	startStr, err := getRequiredParam[string](params, "start")
	if err != nil {
		return nil, err
	}

	endStr, err := getRequiredParam[string](params, "end")
	if err != nil {
		return nil, err
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return nil, fmt.Errorf("invalid start date format: %w", err)
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return nil, fmt.Errorf("invalid end date format: %w", err)
	}

	if end.Before(start) {
		return nil, fmt.Errorf("end date must be after start date")
	}

	// Generate random time between start and end
	diff := end.Unix() - start.Unix()
	randomSeconds := rand.Int63n(diff)
	randomTime := start.Add(time.Duration(randomSeconds) * time.Second)

	return randomTime, nil
}

func (g *DateBetweenGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	startStr, err := getRequiredParam[string](params, "start")
	if err != nil {
		return err
	}

	endStr, err := getRequiredParam[string](params, "end")
	if err != nil {
		return err
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return fmt.Errorf("invalid start date format (use YYYY-MM-DD): %w", err)
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return fmt.Errorf("invalid end date format (use YYYY-MM-DD): %w", err)
	}

	if end.Before(start) {
		return fmt.Errorf("end date must be after start date")
	}

	return nil
}

func NewDecimalGenerator() *DecimalGenerator {
	return &DecimalGenerator{
		BaseGenerator: BaseGenerator{name: "decimal"},
	}
}

func (g *DecimalGenerator) Generate(params map[string]any, column *ColumnInfo) (any, error) {
	min := getParam(params, "min", 0.0)
	max := getParam(params, "max", 1000.0)
	precision := getParam(params, "precision", 2)

	if max <= min {
		return nil, fmt.Errorf("max must be greater than min")
	}

	// Generate random float between min and max
	value := min + rand.Float64()*(max-min)

	// Round to specified precision
	multiplier := float64(1)
	for i := 0; i < precision; i++ {
		multiplier *= 10
	}
	value = float64(int64(value*multiplier)) / multiplier

	return value, nil
}

func (g *DecimalGenerator) Validate(params map[string]any, column *ColumnInfo) error {
	min := getParam(params, "min", 0.0)
	max := getParam(params, "max", 1000.0)

	if max <= min {
		return fmt.Errorf("max (%f) must be greater than min (%f)", max, min)
	}

	return nil
}
