package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	apiURL   string
	tenantID string
)

var rootCmd = &cobra.Command{
	Use:   "tenant-cli",
	Short: "AIbank tenant management CLI",
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := http.Get(apiURL + "/v1/tenants")
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	},
}

var getCmd = &cobra.Command{
	Use:   "get [tenant-id]",
	Short: "Get tenant by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := http.Get(apiURL + "/v1/tenants/" + args[0])
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate [config.yaml]",
	Short: "Validate tenant config YAML against the platform schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("cannot read file: %w", err)
		}
		var config map[string]interface{}
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("invalid YAML: %w", err)
		}
		// Send to tenant-service for validation
		body, _ := json.Marshal(config)
		resp, err := http.Post(apiURL+"/v1/tenants/validate", "application/json", bytes.NewReader(body))
		if err != nil {
			// Offline: just check schema_version exists
			if _, ok := config["schema_version"]; !ok {
				return fmt.Errorf("missing required field: schema_version")
			}
			fmt.Printf("✓ Basic validation passed (offline mode): %s\n", args[0])
			return nil
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 200 {
			fmt.Printf("✓ Valid: %s\n", args[0])
		} else {
			fmt.Fprintf(os.Stderr, "✗ Invalid: %s\n", string(respBody))
			os.Exit(1)
		}
		return nil
	},
}

func main() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", getEnv("TENANT_SERVICE_URL", "http://localhost:8080"), "Tenant service URL")
	rootCmd.AddCommand(listCmd, getCmd, validateCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
