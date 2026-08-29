package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/oasdiff/yaml"
	"github.com/spf13/viper"
)

func GenerateDefaultConfig(v *viper.Viper, outputPath string) error {
	root := make(map[string]any)

	for key := range GetEnvVarMap() {
		value := v.Get(key)

		fmt.Printf(
			"[DEBUG] generate: key=%s value=%#v isSet=%v\n",
			key,
			value,
			v.IsSet(key),
		)

		if value == nil {
			continue
		}

		if err := setNestedValue(root, key, value); err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshal generated config: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("write generated config %q: %w", outputPath, err)
	}

	return nil
}

func setNestedValue(root map[string]any, key string, value any) error {
	parts := strings.Split(key, ".")
	current := root

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return nil
		}

		existing, exists := current[part]
		if !exists {
			child := make(map[string]any)
			current[part] = child
			current = child
			continue
		}

		child, ok := existing.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"config key conflict at %q",
				strings.Join(parts[:i+1], "."),
			)
		}

		current = child
	}

	return nil
}
