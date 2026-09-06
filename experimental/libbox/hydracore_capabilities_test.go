package libbox

import (
	"encoding/json"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/stretchr/testify/require"
)

func TestHydraCoreCapabilities(t *testing.T) {
	t.Parallel()

	content := HydraCoreCapabilities()
	var capabilities hydraCoreCapabilitySet
	require.NoError(t, json.Unmarshal([]byte(content), &capabilities))
	require.Equal(t, 2, capabilities.APIVersion)
	require.Equal(t, "io.hydrabox.hydracore", capabilities.Identity.CoreID)
	require.Equal(t, "HydraCore", capabilities.Identity.CoreName)
	require.Equal(t, C.Version, capabilities.Identity.CoreVersion)
	require.NotEmpty(t, capabilities.Identity.Role)
	require.True(t, capabilities.Features.TargetedURLTest)
	require.True(t, capabilities.Features.ConfigValidation)
	require.True(t, capabilities.Features.RuntimeSnapshot)
	require.True(t, capabilities.Features.RuntimeEvents)
	require.True(t, capabilities.Features.ManagedURLTestSessions)
	require.True(t, capabilities.Features.SubscriptionJWE)
	require.True(t, capabilities.Features.Rmux)
	// The client only offers to raise the level on a live core when this says it will take effect.
	require.True(t, capabilities.Features.RuntimeLogLevel)
	// The client only puts probe_timeout/probe_concurrency into the automatic group's
	// configuration when this says the core will honour them.
	require.True(t, capabilities.Features.URLTestProbeBudget)
	// The workerless edge probe reads its address through HydraCoreTurnEdgeEndpoint.
	require.True(t, capabilities.Features.TurnEdgeEndpoint)
	// A DoH resolver keeps its query string; without the flag the client must neither
	// accept nor emit one.
	require.True(t, capabilities.Features.DNSQuery)
	require.Equal(t, 3, capabilities.Features.AmneziaVersion)
	require.Equal(t, []string{"local", "remote_v2"}, capabilities.ValidationProfiles)
	require.Equal(t, []int{2}, capabilities.SubscriptionContracts)
	require.Equal(t, 2, capabilities.RemotePolicy.Version)
	require.Equal(t, []string{"$schema", "inbounds", "outbounds", "endpoints"}, capabilities.RemotePolicy.SafeTopLevelFields)
	require.Equal(t, []string{"wireguard"}, capabilities.RemotePolicy.SafeEndpointTypes)
	require.Empty(t, capabilities.RemotePolicy.SafeDNSServerTypes)
	require.Empty(t, capabilities.RemotePolicy.SafeProviderTypes)
	require.Contains(t, capabilities.RemotePolicy.ReservedTagPrefixes, "__hydra.")
	require.Equal(t, 2, capabilities.Runtime.Version)
	require.Equal(t, 2, capabilities.Runtime.SnapshotSchemaVersion)
	require.Equal(t, 64, capabilities.Runtime.RetainedURLTestSessions)
	require.Equal(t, hydraCoreCallEnabled, capabilities.Features.Call)
	require.Equal(t, hydraCoreCallEnabled, capabilities.Features.CallVKParasite)
	require.Equal(t, hydraCoreCallEnabled, capabilities.Features.CallVKParasiteQUIC)
	switch capabilities.Identity.Role {
	case "client":
		require.NotContains(t, capabilities.Protocols.Inbounds, "call")
		require.Contains(t, capabilities.Protocols.Outbounds, "call")
		require.True(t, capabilities.Features.CallVKParasiteClient)
		require.False(t, capabilities.Features.CallVKParasiteServer)
		require.Equal(t, []string{"vk"}, capabilities.Protocols.CallPlatforms)
		require.Equal(t, []string{"vk_parasite"}, capabilities.Protocols.CallModes)
	case "vps":
		require.Contains(t, capabilities.Protocols.Inbounds, "call")
		require.NotContains(t, capabilities.Protocols.Outbounds, "call")
		require.False(t, capabilities.Features.CallVKParasiteClient)
		require.True(t, capabilities.Features.CallVKParasiteServer)
		require.Equal(t, []string{"vk"}, capabilities.Protocols.CallPlatforms)
		require.Equal(t, []string{"vk_parasite"}, capabilities.Protocols.CallModes)
	case "combined":
		require.Contains(t, capabilities.Protocols.Inbounds, "call")
		require.Contains(t, capabilities.Protocols.Outbounds, "call")
		require.Equal(t, []string{"dion", "telemost", "vk", "wbstream"}, capabilities.Protocols.CallPlatforms)
		require.Equal(t, []string{"p2p", "vk_parasite"}, capabilities.Protocols.CallModes)
	case "base":
		require.NotContains(t, capabilities.Protocols.Inbounds, "call")
		require.NotContains(t, capabilities.Protocols.Outbounds, "call")
		require.Empty(t, capabilities.Protocols.CallPlatforms)
		require.Empty(t, capabilities.Protocols.CallModes)
	default:
		t.Fatalf("unexpected HydraCore role %q", capabilities.Identity.Role)
	}
}
