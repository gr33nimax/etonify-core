package hydracore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTurnEdgeEndpointLivesInMemoryWithoutAPath(t *testing.T) {
	resetTurnEdgeStore()
	SetTurnEdgeStorePath("")

	RecordTurnEdgeEndpoint("udp://turn.example.invalid:3478", "call-vk", 1)
	require.Equal(t, "udp://turn.example.invalid:3478", TurnEdgeEndpoint())
}

func TestTurnEdgeEndpointSurvivesTheProcessThatRecordedIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn_edge.json")
	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)

	RecordTurnEdgeEndpoint("udp://turn.example.invalid:3478", "call-vk", 1)
	require.FileExists(t, path, "the endpoint must reach the file a later process reads")

	// A later process: no memory, the same store file.
	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)
	require.Equal(t, "udp://turn.example.invalid:3478", TurnEdgeEndpoint())
}

// The reader is usually not the process that recorded the edge. A first empty answer must
// not be remembered as permanent, and a later recording by another process must be visible
// to the next question, not only to the process that wrote it.
func TestTurnEdgeEndpointSeesALaterProcessWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn_edge.json")
	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)

	require.Empty(t, TurnEdgeEndpoint(), "an absent file is no edge, not an error")

	RecordTurnEdgeEndpoint("udp://first.example.invalid:3478", "call-vk", 1)
	require.Equal(t, "udp://first.example.invalid:3478", TurnEdgeEndpoint())

	// Another process rotates the edge underneath this one.
	record := `{"endpoint":"tcp://second.example.invalid:3478","transport_tag":"call-vk-2","runtime_generation":5,"updated_at":2}`
	require.NoError(t, os.WriteFile(path, []byte(record), 0o644))
	require.Equal(t, "tcp://second.example.invalid:3478", TurnEdgeEndpoint(),
		"the first read is not remembered forever")
	attributed := TurnEdgeAttribution()
	require.Equal(t, "call-vk-2", attributed.TransportTag)
	require.Equal(t, uint64(5), attributed.RuntimeGeneration)
}

// The edge record is only an attribution when it says which transport reached the edge
// under which runtime generation. A record from an older core carries neither and must
// read as unconfirmed, so a client files it under nobody.
func TestTurnEdgeAttributionOfALegacyRecordIsUnconfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn_edge.json")
	record := `{"endpoint":"udp://turn.example.invalid:3478","updated_at":2}`
	require.NoError(t, os.WriteFile(path, []byte(record), 0o644))

	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)
	attributed := TurnEdgeAttribution()
	require.Equal(t, "udp://turn.example.invalid:3478", attributed.Endpoint)
	require.Empty(t, attributed.TransportTag, "a legacy record attributes the edge to nobody")
	require.Zero(t, attributed.RuntimeGeneration)
}

func TestTurnEdgeAttributionSurvivesTheProcessThatRecordedIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn_edge.json")
	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)

	RecordTurnEdgeEndpoint("udp://turn.example.invalid:3478", "call-vk", 7)

	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)
	attributed := TurnEdgeAttribution()
	require.Equal(t, "udp://turn.example.invalid:3478", attributed.Endpoint)
	require.Equal(t, "call-vk", attributed.TransportTag)
	require.Equal(t, uint64(7), attributed.RuntimeGeneration)
	require.NotZero(t, attributed.UpdatedAt)
}

func TestTurnEdgeEndpointStaysEmptyWhenNothingWasEverRecorded(t *testing.T) {
	resetTurnEdgeStore()
	SetTurnEdgeStorePath(filepath.Join(t.TempDir(), "absent.json"))
	require.Empty(t, TurnEdgeEndpoint())
}

func TestTurnEdgeEndpointIgnoresACorruptStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn_edge.json")
	require.NoError(t, os.WriteFile(path, []byte("not a store at all"), 0o644))

	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)
	require.Empty(t, TurnEdgeEndpoint())
}

func TestTurnEdgeEndpointIgnoresAnEmptyRecording(t *testing.T) {
	resetTurnEdgeStore()
	SetTurnEdgeStorePath("")

	RecordTurnEdgeEndpoint("", "call-vk", 1)
	require.Empty(t, TurnEdgeEndpoint())
}
