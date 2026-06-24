package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

func main() {
	inputPath := flag.String("input", "docs/swagger.json", "path to the generated Swagger 2 spec")
	outputDir := flag.String("output-dir", "docs-site/api", "directory for OpenAPI 3 output")
	flag.Parse()

	if err := run(*inputPath, *outputDir); err != nil {
		panic(err)
	}
}

func run(inputPath, outputDir string) error {
	rawSpec, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read swagger spec: %w", err)
	}

	var swaggerDoc openapi2.T
	if err := json.Unmarshal(rawSpec, &swaggerDoc); err != nil {
		return fmt.Errorf("parse swagger spec: %w", err)
	}

	openapiDoc, err := openapi2conv.ToV3(&swaggerDoc)
	if err != nil {
		return fmt.Errorf("convert to openapi 3: %w", err)
	}

	if err := openapiDoc.Validate(context.Background()); err != nil {
		return fmt.Errorf("validate openapi 3: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	yamlBytes, err := yaml.Marshal(openapiDoc)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(openapiDoc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "openapi.yaml"), yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write openapi yaml: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "openapi.json"), append(jsonBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("write openapi json: %w", err)
	}

	reference := renderReference(openapiDoc)
	if err := os.WriteFile(filepath.Join(outputDir, "reference.md"), []byte(reference), 0o644); err != nil {
		return fmt.Errorf("write reference markdown: %w", err)
	}

	return nil
}

func renderReference(doc *openapi3.T) string {
	type endpoint struct {
		method      string
		path        string
		summary     string
		description string
	}

	groups := map[string][]endpoint{}
	for _, path := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Find(path)
		if item == nil {
			continue
		}

		for _, operation := range orderedOperations(item) {
			if operation.operation == nil {
				continue
			}

			groupName := groupForOperation(path, operation.operation)
			groups[groupName] = append(groups[groupName], endpoint{
				method:      strings.ToUpper(operation.method),
				path:        path,
				summary:     firstNonEmpty(operation.operation.Summary, operation.operation.OperationID),
				description: strings.TrimSpace(operation.operation.Description),
			})
		}
	}

	groupNames := make([]string, 0, len(groups))
	for groupName := range groups {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)

	var builder strings.Builder
	builder.WriteString("# Endpoint Reference\n\n")
	builder.WriteString("This page is generated from the current OpenAPI 3 document. Regenerate it with `make swagger`.\n\n")

	for _, groupName := range groupNames {
		builder.WriteString("## ")
		builder.WriteString(groupName)
		builder.WriteString("\n\n")
		builder.WriteString("| Method | Path | Summary | Description |\n")
		builder.WriteString("| --- | --- | --- | --- |\n")

		for _, endpoint := range groups[groupName] {
			builder.WriteString("| ")
			builder.WriteString(endpoint.method)
			builder.WriteString(" | ")
			builder.WriteString(escapeTable(endpoint.path))
			builder.WriteString(" | ")
			builder.WriteString(escapeTable(endpoint.summary))
			builder.WriteString(" | ")
			builder.WriteString(escapeTable(endpoint.description))
			builder.WriteString(" |\n")
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

type orderedOperation struct {
	method    string
	operation *openapi3.Operation
}

func orderedOperations(item *openapi3.PathItem) []orderedOperation {
	operations := []orderedOperation{
		{method: "get", operation: item.Get},
		{method: "post", operation: item.Post},
		{method: "put", operation: item.Put},
		{method: "patch", operation: item.Patch},
		{method: "delete", operation: item.Delete},
		{method: "head", operation: item.Head},
		{method: "options", operation: item.Options},
		{method: "trace", operation: item.Trace},
	}

	filtered := make([]orderedOperation, 0, len(operations))
	for _, operation := range operations {
		if operation.operation != nil {
			filtered = append(filtered, operation)
		}
	}

	return filtered
}

func groupForOperation(path string, operation *openapi3.Operation) string {
	if len(operation.Tags) > 0 && strings.TrimSpace(operation.Tags[0]) != "" {
		return titleCase(operation.Tags[0])
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "General"
	}

	switch segments[0] {
	case "auth":
		return "Authentication"
	case "api_keys":
		return "API Keys"
	case "files", "folders":
		return "Files"
	case "share", "d":
		return "Sharing"
	case "user":
		return "User"
	case "workspaces":
		return "Workspaces"
	default:
		return titleCase(segments[0])
	}
}

func titleCase(value string) string {
	parts := strings.Split(strings.ReplaceAll(value, "_", " "), " ")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
