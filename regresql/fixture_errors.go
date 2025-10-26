package regresql

import "fmt"

// FixtureError represents an error related to fixture operations
type FixtureError struct {
	Message string
	Cause   error
}

func (e *FixtureError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *FixtureError) Unwrap() error {
	return e.Cause
}

// ErrFixtureNotFound creates an error for missing fixtures
func ErrFixtureNotFound(name string) error {
	return &FixtureError{
		Message: fmt.Sprintf("fixture not found: %s", name),
	}
}

// ErrInvalidFixture creates an error for invalid fixture definitions
func ErrInvalidFixture(format string, args ...interface{}) error {
	return &FixtureError{
		Message: fmt.Sprintf("invalid fixture: "+format, args...),
	}
}

// ErrCircularDependency creates an error for circular fixture dependencies
func ErrCircularDependency(path []string) error {
	return &FixtureError{
		Message: fmt.Sprintf("circular dependency detected: %v", path),
	}
}

// ErrFixtureApplication creates an error for fixture application failures
func ErrFixtureApplication(fixtureName string, cause error) error {
	return &FixtureError{
		Message: fmt.Sprintf("failed to apply fixture '%s'", fixtureName),
		Cause:   cause,
	}
}

// ErrGeneratorNotFound creates an error for missing generators
func ErrGeneratorNotFound(name string) error {
	return &FixtureError{
		Message: fmt.Sprintf("generator not found: %s", name),
	}
}

// ErrSchemaIntrospection creates an error for schema introspection failures
func ErrSchemaIntrospection(cause error) error {
	return &FixtureError{
		Message: "failed to introspect database schema",
		Cause:   cause,
	}
}
