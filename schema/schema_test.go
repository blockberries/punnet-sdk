package schema_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSchemaFilesExist verifies all expected schema files exist
func TestSchemaFilesExist(t *testing.T) {
	schemaFiles := []string{
		"types.cram",
		"auth.cram",
		"bank.cram",
		"staking.cram",
	}

	for _, file := range schemaFiles {
		path := filepath.Join(".", file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Schema file %s does not exist", file)
		}
	}
}

// TestSchemaFilesHavePackage verifies all schema files declare a package
func TestSchemaFilesHavePackage(t *testing.T) {
	schemaFiles := []string{
		"types.cram",
		"auth.cram",
		"bank.cram",
		"staking.cram",
	}

	for _, file := range schemaFiles {
		path := filepath.Join(".", file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}

		if !strings.Contains(string(content), "package punnet.") {
			t.Errorf("Schema file %s does not declare a package", file)
		}

		if !strings.Contains(string(content), "syntax = \"proto3\"") {
			t.Errorf("Schema file %s does not declare proto3 syntax", file)
		}
	}
}

// TestSchemaFieldNumbering verifies field numbering conventions
func TestSchemaFieldNumbering(t *testing.T) {
	schemaFiles := []string{
		"types.cram",
		"auth.cram",
		"bank.cram",
		"staking.cram",
	}

	for _, file := range schemaFiles {
		path := filepath.Join(".", file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Skip lines that are not field definitions
			if strings.HasPrefix(trimmed, "//") ||
				strings.HasPrefix(trimmed, "syntax") ||
				strings.HasPrefix(trimmed, "package") ||
				strings.HasPrefix(trimmed, "option") ||
				strings.HasPrefix(trimmed, "import") ||
				strings.HasPrefix(trimmed, "message") {
				continue
			}

			// Check for field definitions (lines with "= N;")
			if strings.Contains(line, " = ") && strings.Contains(line, ";") {
				// Extract field number
				parts := strings.Split(line, " = ")
				if len(parts) >= 2 {
					numPart := strings.TrimSpace(strings.Split(parts[1], ";")[0])

					// Field number should be numeric
					if numPart == "" || !isNumeric(numPart) {
						t.Errorf("%s line %d: invalid field number format", file, i+1)
					}

					// Field numbers should start at 1
					if numPart == "0" {
						t.Errorf("%s line %d: field numbers should start at 1", file, i+1)
					}
				}
			}
		}
	}
}

// TestSchemaDocumentation verifies all messages have documentation
func TestSchemaDocumentation(t *testing.T) {
	schemaFiles := []string{
		"types.cram",
		"auth.cram",
		"bank.cram",
		"staking.cram",
	}

	for _, file := range schemaFiles {
		path := filepath.Join(".", file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}

		lines := strings.Split(string(content), "\n")
		messageName := ""

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Track message definitions
			if strings.HasPrefix(trimmed, "message ") {
				parts := strings.Split(trimmed, " ")
				if len(parts) >= 2 {
					messageName = strings.TrimSuffix(parts[1], " {")
				}
			}

			// Check that message definitions have comments before them
			if strings.HasPrefix(trimmed, "message ") && i > 0 {
				prevLine := strings.TrimSpace(lines[i-1])
				if !strings.HasPrefix(prevLine, "//") {
					t.Logf("Warning: %s line %d: message %s missing documentation comment",
						file, i+1, messageName)
				}
			}
		}
	}
}

// TestSchemaImports verifies import statements
func TestSchemaImports(t *testing.T) {
	// Auth, bank, and staking should import types.cram
	moduleSchemasWithImports := []string{
		"auth.cram",
		"bank.cram",
		"staking.cram",
	}

	for _, file := range moduleSchemasWithImports {
		path := filepath.Join(".", file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}

		if !strings.Contains(string(content), "import \"types.cram\"") {
			t.Errorf("Schema file %s should import types.cram", file)
		}
	}
}

// TestTypesSchemaCompleteness verifies core types are defined
func TestTypesSchemaCompleteness(t *testing.T) {
	path := filepath.Join(".", "types.cram")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read types.cram: %v", err)
	}

	requiredTypes := []string{
		"message Account",
		"message Authority",
		"message Authorization",
		"message Signature",
		"message Coin",
		"message Transaction",
		"message ValidatorUpdate",
		"message TxResult",
		"message Event",
	}

	contentStr := string(content)
	for _, reqType := range requiredTypes {
		if !strings.Contains(contentStr, reqType) {
			t.Errorf("types.cram missing required type: %s", reqType)
		}
	}
}

// TestAuthSchemaCompleteness verifies auth messages are defined
func TestAuthSchemaCompleteness(t *testing.T) {
	path := filepath.Join(".", "auth.cram")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read auth.cram: %v", err)
	}

	requiredMessages := []string{
		"message MsgCreateAccount",
		"message MsgUpdateAuthority",
		"message MsgDeleteAccount",
	}

	contentStr := string(content)
	for _, reqMsg := range requiredMessages {
		if !strings.Contains(contentStr, reqMsg) {
			t.Errorf("auth.cram missing required message: %s", reqMsg)
		}
	}
}

// TestBankSchemaCompleteness verifies bank messages are defined
func TestBankSchemaCompleteness(t *testing.T) {
	path := filepath.Join(".", "bank.cram")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read bank.cram: %v", err)
	}

	requiredMessages := []string{
		"message MsgSend",
		"message MsgMultiSend",
		"message Input",
		"message Output",
		"message Balance",
	}

	contentStr := string(content)
	for _, reqMsg := range requiredMessages {
		if !strings.Contains(contentStr, reqMsg) {
			t.Errorf("bank.cram missing required message: %s", reqMsg)
		}
	}
}

// TestStakingSchemaCompleteness verifies staking messages are defined
func TestStakingSchemaCompleteness(t *testing.T) {
	path := filepath.Join(".", "staking.cram")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read staking.cram: %v", err)
	}

	requiredMessages := []string{
		"message MsgCreateValidator",
		"message MsgDelegate",
		"message MsgUndelegate",
		"message Validator",
		"message Delegation",
	}

	contentStr := string(content)
	for _, reqMsg := range requiredMessages {
		if !strings.Contains(contentStr, reqMsg) {
			t.Errorf("staking.cram missing required message: %s", reqMsg)
		}
	}
}

// TestGoPackageOption verifies go_package option is set
func TestGoPackageOption(t *testing.T) {
	schemaFiles := map[string]string{
		"types.cram":   "github.com/blockberries/punnet-sdk/types/generated",
		"auth.cram":    "github.com/blockberries/punnet-sdk/modules/auth/generated",
		"bank.cram":    "github.com/blockberries/punnet-sdk/modules/bank/generated",
		"staking.cram": "github.com/blockberries/punnet-sdk/modules/staking/generated",
	}

	for file, expectedPackage := range schemaFiles {
		path := filepath.Join(".", file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}

		expectedOption := "option go_package = \"" + expectedPackage + "\""
		if !strings.Contains(string(content), expectedOption) {
			t.Errorf("%s missing correct go_package option. Expected: %s", file, expectedOption)
		}
	}
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
