package regresql

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrUserQuit is returned when the user chooses to quit during interactive mode
var ErrUserQuit = errors.New("user quit")

// InteractivePrompter handles interactive user prompts for update operations
type InteractivePrompter struct {
	reader *bufio.Reader
}

// NewInteractivePrompter creates a new interactive prompter
func NewInteractivePrompter() *InteractivePrompter {
	return &InteractivePrompter{reader: bufio.NewReader(os.Stdin)}
}

// PromptAccept shows query name and asks user to accept/skip/quit
// Returns: "accept", "skip", "quit"
func (p *InteractivePrompter) PromptAccept(queryName string, diff string) string {
	fmt.Printf("\nQuery: %s\n", queryName)
	if diff != "" {
		fmt.Printf("%s\n", diff)
	}
	fmt.Print("[a]ccept / [s]kip / [q]uit: ")

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	switch input {
	case "a", "accept", "":
		return "accept"
	case "q", "quit":
		return "quit"
	default:
		return "skip"
	}
}
