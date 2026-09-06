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

	RecordTurnEdgeEndpoint("udp://turn.example.invalid:3478")
	require.Equal(t, "udp://turn.example.invalid:3478", TurnEdgeEndpoint())
}

func TestTurnEdgeEndpointSurvivesTheProcessThatRecordedIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn_edge.json")
	resetTurnEdgeStore()
	SetTurnEdgeStorePath(path)

	RecordTurnEdgeEndpoint("udp://turn.example.invalid:3478")
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

	RecordTurnEdgeEndpoint("udp://first.example.invalid:3478")
	require.Equal(t, "udp://first.example.invalid:3478", TurnEdgeEndpoint())

	// Another process rotates the edge underneath this one.
	record := `{"endpoint":"tcp://second.example.invalid:3478","updated_at":2}`
	require.NoError(t, os.WriteFile(path, []byte(record), 0o644))
	require.Equal(t, "tcp://second.example.invalid:3478", TurnEdgeEndpoint(),
		"the first read is not remembered forever")
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

	RecordTurnEdgeEndpoint("")
	require.Empty(t, TurnEdgeEndpoint())
}
