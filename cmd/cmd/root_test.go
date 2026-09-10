package cmd
package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newFlagOverrideTestCommand mirrors the relevant root command flags so the
// override helpers can be exercised without executing the full backend.
func newFlagOverrideTestCommand() *cobra.Command {
	testCmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}

	testCmd.Flags().Bool("github-auth", false, "")
	testCmd.Flags().Bool("google-auth", false, "")
	testCmd.Flags().Bool("password-auth", false, "")
	testCmd.Flags().Bool("admin-server", false, "")
	testCmd.Flags().StringSlice("email-whitelist", nil, "")

	return testCmd
}

func TestApplyAuthFlagOverridesEmailWhitelist(t *testing.T) {
	t.Run("sets whitelist when flag is changed", func(t *testing.T) {
		testCmd := newFlagOverrideTestCommand()
		if err := testCmd.Flags().Set("email-whitelist", "One@Example.com, two@example.com"); err != nil {
			t.Fatalf("set flag: %v", err)
		}

		v := viper.New()
		v.SetDefault("api.email_whitelist", []string{})

		applyAuthFlagOverrides(testCmd, v)

		got := v.GetStringSlice("api.email_whitelist")
		want := []string{"One@Example.com", " two@example.com"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("api.email_whitelist = %#v, want %#v", got, want)
		}
	})

	t.Run("leaves config untouched when flag is not changed", func(t *testing.T) {
		testCmd := newFlagOverrideTestCommand()

		v := viper.New()
		v.SetDefault("api.email_whitelist", []string{"preset@example.com"})

		applyAuthFlagOverrides(testCmd, v)

		got := v.GetStringSlice("api.email_whitelist")
		want := []string{"preset@example.com"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("api.email_whitelist = %#v, want %#v", got, want)
		}
	})
}

func TestApplyAuthFlagOverridesAuthProviders(t *testing.T) {
	testCmd := newFlagOverrideTestCommand()
	if err := testCmd.Flags().Set("github-auth", "true"); err != nil {
		t.Fatalf("set github-auth: %v", err)
	}

	v := viper.New()
	v.SetDefault("api.enable_github_auth", false)

	applyAuthFlagOverrides(testCmd, v)

	if !v.GetBool("api.enable_github_auth") {
		t.Fatal("api.enable_github_auth should be true after override")
	}
	if v.GetBool("api.enable_google_auth") {
		t.Fatal("api.enable_google_auth should remain false")
	}
}

func TestApplyAdminFlagOverrides(t *testing.T) {
	testCmd := newFlagOverrideTestCommand()
	if err := testCmd.Flags().Set("admin-server", "true"); err != nil {
		t.Fatalf("set admin-server: %v", err)
	}

	v := viper.New()
	v.SetDefault("admin.enabled", false)

	applyAdminFlagOverrides(testCmd, v)

	if !v.GetBool("admin.enabled") {
		t.Fatal("admin.enabled should be true after override")
	}
}
