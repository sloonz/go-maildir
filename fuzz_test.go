package maildir

import "testing"

// This does not run the fuzz test, only tests the fuzz function
func TestFuzz(t *testing.T) {
	Fuzz([]byte("Résumé"))
}