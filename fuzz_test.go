package maildir

import "testing"

func TestFuzz(t *testing.T) {
	Fuzz([]byte("Résumé"))
}