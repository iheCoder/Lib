package consistent_hash

import (
	"testing"
)

func TestHashRing(t *testing.T) {
	ring := NewHashRing(3)

	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	keys := []string{"apple", "banana", "cherry", "date", "fig", "grape"}

	for _, key := range keys {
		node := ring.GetNode(key)
		if node == "" {
			t.Errorf("key %s was not assigned to any node", key)
		} else {
			t.Logf("key %s is assigned to %s", key, node)
		}
	}
}
