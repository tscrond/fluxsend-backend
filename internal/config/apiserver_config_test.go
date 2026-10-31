package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustedProxyCIDRValues(t *testing.T) {
	v := viper.New()
	v.Set("api.trusted_proxy_cidrs", "172.18.0.0/16, 10.1.2.3")

	got, err := trustedProxyCIDRValues(v, "api.trusted_proxy_cidrs")

	require.NoError(t, err)
	assert.Equal(t, []string{"172.18.0.0/16", "10.1.2.3/32"}, got)
}

func TestTrustedProxyCIDRValuesRejectsInvalidAddress(t *testing.T) {
	v := viper.New()
	v.Set("api.trusted_proxy_cidrs", "not-an-ip")

	_, err := trustedProxyCIDRValues(v, "api.trusted_proxy_cidrs")

	require.ErrorContains(t, err, `invalid api.trusted_proxy_cidrs value "not-an-ip"`)
}
